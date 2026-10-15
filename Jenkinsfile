pipeline {
  agent any
  stages {
    stage('SCM') {
      steps {
        checkout scm
      }
    }
    stage('SonarQube Analysis') {
        steps {
            // Inyecta las variables del servidor SonarQube configurado en Jenkins
            withSonarQubeEnv('SonarQube') {
                sh 'sonar-scanner'
            }
        }
    }
    stage('Build docker image') {
      steps {
        
        sh 'docker build -t ghcr.io/dsaub/poketeamcalculator:latest .'
        withCredentials([usernamePassword(
          credentialsId: 'github-pat',
          usernameVariable: 'GH_USER',
          passwordVariable: 'GH_TOKEN'
        )]) {
          sh '''
            echo "$GH_TOKEN" | docker login ghcr.io -u "$GH_USER" --password-stdin
            docker push ghcr.io/dsaub/poketeamcalculator:latest
            docker logout ghcr.io
          '''
        }
      }
    }
  }
  post {
    always {
      cleanWs()
    }
  }
}
