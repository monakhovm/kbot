pipeline {
    agent any

    parameters {
        choice(
            name: 'TARGETOS',
            choices: ['linux', 'darwin', 'windows'],
            description: 'Target OS (linux, darwin, windows)'
        )
        choice(
            name: 'TARGETARCH',
            choices: ['amd64', 'arm64'],
            description: 'Target architecture (amd64, arm64)'
        )
        booleanParam(
            name: 'SKIP_TESTS',
            defaultValue: false,
            description: 'Skip tests?'
        )
        booleanParam(
            name: 'SKIP_LINT',
            defaultValue: false,
            description: 'Skip linting?'
        )
    }

    environment {
        REPO = 'https://github.com/monakhovm/kbot.git'
        BRANCH = 'main'
    }

    stages {
        stage('Checkout') {
            steps {
                git branch: "${env.BRANCH}", url: "${env.REPO}"
            }
        }

        stage('Test') {
            steps {
                script {
                    if (params.SKIP_TESTS == false) {
                        sh 'make test'
                    } else {
                        echo 'Tests skipped'
                    }
                }
            }
        }

        stage('Build') {
            steps {
                sh "make build TARGETOS=${params.TARGETOS} TARGETARCH=${params.TARGETARCH}"
            }
        }

        stage('Image') {
            steps {
                sh "make image TARGETOS=${params.TARGETOS} TARGETARCH=${params.TARGETARCH}"
            }
        }

        stage('Push') {
            steps {
                sh "make push TARGETOS=${params.TARGETOS} TARGETARCH=${params.TARGETARCH}"
            }
        }
    }
}
