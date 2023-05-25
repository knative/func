const axios = require('axios')
const xml2js = require('xml2js');
const {Octokit} = require("octokit");
const {readFile,writeFile} = require('fs/promises');
const {spawn} = require('node:child_process');

const cePomPath = "templates/springboot/cloudevents/pom.xml"
const httpPomPath = "templates/springboot/http/pom.xml"
const octokit = new Octokit({auth: process.env.GITHUB_TOKEN});
const [owner, repo] = process.env.GITHUB_REPOSITORY.split('/')

const getLatestPlatform = async () => {
    const resp = await axios.get("https://api.github.com/repos/spring-projects/spring-boot/releases/latest")
    return (resp.data.draft === 'false') ? resp.data.name.replace(/[A-Za-z]/g, "") : null;
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
    return pom.project.properties[0]['springboot.platform.version'][0]
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
    const newPomData = pomData.replace(new RegExp('<springboot.platform.version>[\\w.]+</springboot.platform.version>', 'i'),
        `<springboot.platform.version>${newPlatform}</springboot.platform.version>`)
    await writeFile(pomPath, newPomData)
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
    const prTitle = `chore: update Springboot platform version to ${latestPlatform}`
    const branchName = `update-springboot-platform-${latestPlatform}`
    const cePlatform = await platformFromPom(cePomPath)
    const httpPlatform = await platformFromPom(httpPomPath)

    if(latestPlatform === null) {
        console.log("Spring Boot platform latest version is not ready to use!")
        return 
    }

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
