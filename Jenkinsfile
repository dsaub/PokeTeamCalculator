@Library('mlib') _
pipeline {
  agent any
  stages {
    stage('SCM') {
      steps {
        checkout scm
      }
    }
    stage('Security Checks') {
      steps {
        scanSecrets()
        lintDockerfile()
      }
    }
    stage('SonarQube Analysis') {
        steps {
            runSonar()
        }
    }
    stage('Preparar emulación x86_64') {
      steps {
        // Registra QEMU en binfmt_misc para poder ejecutar binarios amd64 en el agente ARM
        sh 'docker run --privileged --rm tonistiigi/binfmt:latest --install amd64'
      }
    }
    stage('Build docker image') {
      steps {
        sh '''
          docker buildx build \
            --platform linux/amd64,linux/arm64 \
            --load \
            -t ghcr.io/dsaub/poketeamcalculator:latest \
            .
        '''
      }
    }
    stage("Escaneo de Seguridad de Trivy") {
      steps {
        scanImage('ghcr.io/dsaub/poketeamcalculator:latest')
      }
    }
    stage("Subir imagen") {
      steps {
        dockerPushGhcr('ghcr.io/dsaub/poketeamcalculator:latest')
      }
    }
  }
  post {
    always {
      cleanWs()
    }
  }
}
