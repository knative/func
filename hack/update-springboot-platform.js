const axios = require('axios')
const xml2js = require('xml2js');
const yaml = require('yaml')
const semver = require('semver')
const {Octokit} = require("octokit");
const {readFile,writeFile} = require('fs/promises');
const {spawn} = require('node:child_process');

const cePomPath = "templates/springboot/cloudevents/pom.xml"
const httpPomPath = "templates/springboot/http/pom.xml"
const octokit = new Octokit({auth: process.env.GITHUB_TOKEN});
const [owner, repo] = process.env.GITHUB_REPOSITORY.split('/')

const getLatestPlatform = async () => {
    const resp = await axios.get("https://api.github.com/repos/spring-projects/spring-boot/releases/latest")
    return (resp.data.draft === false) ? resp.data.name.replace(/[A-Za-z]/g, "") : null;
}

const prExists = async (pred) => {

    let page = 1
    const perPage = 10;

    while (true) {
        const resp = await octokit.rest.pulls.list({
            owner: owner,
            repo: repo,
            state: 'open',
            per_page: perPage,
            page: page
        })

        for (const e of resp.data) {
            if (pred(e)) {
                return true
            }
        }
        if (resp.data.length < perPage) {
            return false
        }
        page++
    }
}

const parseXML = (text) => new Promise((resolve, reject) => {
    xml2js.parseString(text, {}, (err, res) => {
        if (err) {
            reject(err)
        }
        resolve(res)
    })
})

const platformFromPom = async (pomPath) => {
    const pomData = await readFile(pomPath, {encoding: 'utf8'});
    const pom = await parseXML(pomData)
    return pom.project.parent[0].version[0]
}

const prepareBranch = async (branchName, prTitle) => {
    const script = `git config user.email "automation@knative.team" && \\
  git config user.name "Knative Automation" && \\
  git checkout -b "${branchName}" && \\
  make generate/zz_filesystem_generated.go && \\
  git add "${cePomPath}" "${httpPomPath}" generate/zz_filesystem_generated.go && \\
  git commit -m "${prTitle}" && \\
  git push --set-upstream origin "${branchName}"
`
    const subproc = spawn("sh", ["-c", script], {stdio: ['inherit', 'inherit', 'inherit']})

    return new Promise((resolve, reject) => {
        subproc.on('exit', code => {
            if (code === 0) {
                resolve()
                return
            }
            reject(new Error("cannot prepare branch: non-zero exit code"))
        })
    })
}

const updatePlatformInPom = async (pomPath, newPlatform) => {
    const pomData = await readFile(pomPath, {encoding: 'utf8'});
    const pom = await parseXML(pomData)
    pom.project.parent[0].version[0] = newPlatform

    const compatibleSpringCloudVersion = await getCompatibleSpringCloudVersion(newPlatform)
    pom.project.properties[0]['spring-cloud.version'] = [compatibleSpringCloudVersion]

    const builder = new xml2js.Builder( { headless: false, renderOpts: { pretty: true }  })
    const newPomData = builder.buildObject(pom)
    await writeFile(pomPath, newPomData)
}

const getCompatibleSpringCloudVersion = async (newPlatform) => {
    const bomUrl = "https://raw.githubusercontent.com/spring-io/start.spring.io/main/start-site/src/main/resources/application.yml"
    const mappings = yaml.parseAllDocuments((await axios.get(bomUrl)).data)[0].toJS()
        .initializr
        .env
        .boms['spring-cloud']
        .mappings

    const newPlatformVersion = semver.parse(newPlatform, {}, true)
    for (const {compatibilityRange, version} of mappings) {
        let begin, end
        if (compatibilityRange.startsWith('[')) {
            let [b, e] = compatibilityRange.slice(1, -1).split(',')
            begin = semver.parse(b, {}, true)
            end = semver.parse(e, {}, true)
        } else {
            begin = semver.parse(compatibilityRange, {}, true)
            end = semver.parse("999.999.999", {}, true)
        }

        if (newPlatformVersion.compare(begin) >= 0 && newPlatformVersion.compare(end) < 0) {
            return version
        }
    }
    throw new Error("cannot get latest compatible spring-cloud version")
}

const smokeTest = () => {
    const subproc = spawn("make", ["test-springboot"], {stdio: ['inherit', 'inherit', 'inherit']})
    return new Promise((resolve, reject) => {
        subproc.on('exit', code => {
            if (code === 0) {
                resolve()
                return
            }
            reject(new Error("smoke test failed: non-zero exit code"))
        })
    })
}

const main = async () => {
    const latestPlatform = await getLatestPlatform()

    if(latestPlatform === null) {
        console.log("Spring Boot platform latest version is not ready to use!")
        return
    }

    const prTitle = `chore: update Springboot platform version to ${latestPlatform}`
    const branchName = `update-springboot-platform-${latestPlatform}`
    const cePlatform = await platformFromPom(cePomPath)
    const httpPlatform = await platformFromPom(httpPomPath)

    if (cePlatform === latestPlatform && httpPlatform === latestPlatform) {
        console.log("Spring Boot platform is up-to-date!")
        return
    }

    if (await prExists(({title}) => title === prTitle)) {
        console.log("The PR already exists!")
        return
    }

    await updatePlatformInPom(cePomPath, latestPlatform)
    await updatePlatformInPom(httpPomPath, latestPlatform)
    await smokeTest()
    await prepareBranch(branchName, prTitle)
    await octokit.rest.pulls.create({
        owner: owner,
        repo: repo,
        title: prTitle,
        body: prTitle,
        base: 'main',
        head: `${owner}:${branchName}`,
    })
    console.log("The PR has been created!")

}

main().then(value => {
    console.log("OK!")
}).catch(reason => {
    console.log("ERROR: ", reason)
    process.exit(1)
})
