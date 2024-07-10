// const xml2js = require('xml2js');
const {Octokit} = require("octokit");
// const {readFile,writeFile} = require('fs/promises');
const {spawn} = require('node:child_process');

const octokit = new Octokit({auth: process.env.GITHUB_TOKEN});
const [owner, repo] = process.env.GITHUB_REPOSITORY.split('/')

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

/**
 * @param script
 * @return {Promise<number>}
 */
const runScript = async (script) => {
    const subproc = spawn("sh", ["-c", script], {stdio: ['inherit', 'inherit', 'inherit']})
    return new Promise((resolve, reject) => {
        subproc.on('exit', code => {
            resolve(code)
        })
        if (typeof subproc.exitCode === 'number') {
            resolve(subproc.exitCode)
        }
    })
}

/**
 * @return {Promise<boolean>}
 */
const updateCA = async () => {
    let ec = await runScript('make templates/certs/ca-certificates.crt')
    if (ec !== 0) {
        throw new Error('cannot update CA bundle')
    }
    return (await runScript('git diff --exit-code -- templates/certs/ca-certificates.crt')) !== 0
}

const prepareBranch = async (branchName, prTitle) => {
    const script = `git config user.email "automation@knative.team" && \\
  git config user.name "Knative Automation" && \\
  git checkout -b "${branchName}" && \\
  make generate/zz_filesystem_generated.go && \\
  git add generate/zz_filesystem_generated.go templates/certs/ca-certificates.crt && \\
  git commit -m "${prTitle}" && \\
  git push --set-upstream origin "${branchName}"
`
    const ec = await runScript(script)
    if (ec !== 0) {
        throw new Error("cannot prepare branch: non-zero exit code")
    }
}

const main = async () => {
    const prTitle = `chore: update CA bundle`
    if (await prExists(({title}) => title === prTitle)) {
        console.log("The PR already exists!")
        return
    }

    const hasUpdated = await updateCA()
    if (!hasUpdated) {
        console.log('The CA bundle is up to date. Nothing to be done.')
        return
    }

    const branchName = `update-ca-bundle-${(new Date()).toISOString().split('T')[0]}`

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
