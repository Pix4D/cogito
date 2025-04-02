You can either generate an app for your indivdual account, or create a github organization and assign that as the application owner

1. Create a test repo
https://github.com/new
2. Create a new github app
https://github.com/settings/apps/new
* Mark webhook as inactive)
* Add repoistory permission: Commit Statuses: Read/write
3. Generate a private key and download a copy

Add these to a file source_app.sh

$ cat source_app.sh
export COGITO_TEST_INSTALLATION_ID=<installation id>
export COGITO_TEST_APPLICATION_ID=<application id>
export COGITO_TEST_COMMIT_SHA=<amy commit sha on the test repo>
export COGITO_TEST_USE_GITHUB_APP=true
export COGITO_TEST_REPO_NAME=<repo name>
export COGITO_TEST_REPO_OWNER=<your username or the organization name)
export COGITO_TEST_PRIVATE_KEY=$(cat <private key path>)

$ source source_env.sh  && go test -v ./..

TODO:
* get tests passing in root locally
* attemp to make changes to taskfile
* suggest how to add to keeppass
