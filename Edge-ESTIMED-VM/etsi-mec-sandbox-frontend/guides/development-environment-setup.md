# Development Environment Setup

MEC Sandbox development environment installation procedures can be performed manually or automatically.

* To install **manually** \- Read through the following below sections
* To install using **Ansible** - follow this link

## Ubuntu

See [Ubuntu runtime setup](runtime-environment-setup.md#ubuntu)

## Go Toolchain

We use the official [Go Programming Language install procedure](https://golang.org/doc/install)

Versions we use:

* 1.26.5

How we do it:

### STEP 1 - Download Go linux tarball

Download the tarball from [here](https://golang.org/dl/)

### STEP 2 - Unzip tarball & install

```bash
# Example tarball: go1.26.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.26.5.linux-amd64.tar.gz
```

### STEP 3 - Create Go working directory

```bash
mkdir -p ~/gocode/bin
```

### STEP 4 - Update PATH in bashrc

```bash
# Add the following lines at the end of your $HOME/.bashrc
export GOPATH=$HOME/gocode
export PATH=$PATH:$GOPATH/bin:/usr/local/go/bin
```

### STEP 5 - Apply profile updates

```bash
source ~/.bashrc
```

### STEP 6 - Verify install

```bash
which go
go version

# Example output:
#   /usr/local/go/bin/go
#   go version go1.26.5 linux/amd64
```

## Node.js & npm

We use the _How To Install Using NVM_ method. NVM is the _Node Version Manager (NVM)_ tool used to install Node.js. It allows concurrent use of different Node.js installations and simplifies the version upgrade procedure.

Versions we use:

* Node.js: 24.18.0
* npm: 12.0.1

How we do it:

### STEP 1 - Install dependencies

```bash
sudo apt-get update
sudo apt-get install build-essential libssl-dev
```

### STEP 2 - Download & install NVM

```bash
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.1/install.sh | bash
```

### STEP 3 - Apply profile updates

```bash
source ~/.bashrc
```

### STEP 4 - Install Node.js

```bash
nvm install 24.18.0
```

### STEP 5 - Update NPM version bundled with Node.js

```bash
npm install -g npm@12.0.1
```

### STEP 6 - Verify install

```bash
node -v
npm -v

# Example output:
#   v24.18.0
#   12.0.1
```

## Linters

The linting tools used for development are:

* ESLint
* golangci-lint

### ESLint

[ESLint](https://eslint.org/) is a pluggable linting utility for JavaScript.

Versions we use:

* 9.39.5

How we do it:

#### STEP 1 - Install ESLint globally

```bash
npm install -g eslint@9.39.5
```

#### STEP 2 - Verify install

```bash
eslint -v

# Example output:
#   v9.39.5
```

### GolangCI-Lint

[GolangCI-Lint](https://golangci-lint.run/) is a linters aggregator for Go.

Versions we use:

* 2.12.2

How we do it:

#### STEP 1 - Install GolangCI-Lint

```bash
# binary will be $(go env GOPATH)/bin/golangci-lint
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v2.12.2
```

#### STEP 2 - Verify install

```bash
golangci-lint --version

# Example output:
# golangci-lint has version 2.12.2 built from ...
```

## IDE

There is no strict requirement on which IDE to use for development. We use **Visual Studio Code**.

### Visual Studio Code

[Visual Studio Code](https://code.visualstudio.com/) is ‘a lightweight but powerful source code editor which runs on your desktop.’

Extensions we use:

* ESLint
* Go

Settings we use:

```json
{
    "files.eol": "\n",
    "go.formatTool": "goimports",
    "go.lintTool": "golangci-lint",
    "go.lintFlags": [
        "--fast"
    ],
    "go.useLanguageServer": false,
    "go.testOnSave": true,
    "[go]": {
        "editor.snippetSuggestions": "none",
        "editor.formatOnSave": true,
        "editor.codeActionsOnSave": {
            "source.organizeImports": true
        }
    },
    "[javascript]": {
        "editor.codeActionsOnSave": {
            "source.fixAll.eslint": true
        }
    },
    "eslint.format.enable": true
}
```
