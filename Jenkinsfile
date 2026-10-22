@Library('mlib') _

// ID del sandbox x86_64 de Daytona. Se crea justo antes de construir y se
// borra siempre en el post, haya fallado el build o no.
def sandbox

// Imágenes sin etiqueta: buildMultiArchImage publica :latest y :<commit>.
def IMAGENES = [
  api   : [image: 'ghcr.io/dsaub/poketeamcalculator',        context: '.',      dockerfile: 'Dockerfile'],
  worker: [image: 'ghcr.io/dsaub/poketeamcalculator-worker', context: 'worker', dockerfile: 'worker/Dockerfile'],
  web   : [image: 'ghcr.io/dsaub/poketeamcalculator-web',    context: 'web',    dockerfile: 'web/Dockerfile'],
]

pipeline {
  // buildMultiArchImage reserva sus propios agentes: el pipeline no ocupa
  // ninguno y cada stage que lo necesita pide srv01.
  agent none

  environment {
    REGISTRO_CREDENCIAL = 'github-pat'
  }

  stages {
    // Los tres componentes se analizan a la vez. Cada rama usa su propio
    // sonar-project.properties y su propio Dockerfile.
    stage('Análisis de seguridad (paralelo)') {
      agent { label 'srv01' }
      steps {
        checkout scm
        script {
          parallel(
            secretos: {
              // Gitleaks sobre todo el repositorio (incluye api/, worker/ y web/).
              scanSecrets()
            },
            api: {
              dir('api') {
                runSonar()
              }
              lintDockerfile('Dockerfile')
            },
            worker: {
              dir('worker') {
                runSonar()
              }
              lintDockerfile('worker/Dockerfile')
            },
            web: {
              dir('web') {
                runSonar()
              }
              lintDockerfile('web/Dockerfile')
            }
          )
        }
      }
      post {
        always {
          cleanWs()
        }
      }
    }

    // Sustituye a la emulación QEMU: linux/amd64 se construye en un sandbox
    // x86_64 de Daytona (credencial Secret text 'daytona-token').
    stage('Preparar sandbox Daytona') {
      agent { label 'srv01' }
      steps {
        script {
          // Las imágenes amd64 se construyen de una en una en este sandbox. Daytona
          // no admite recursos al crear desde un snapshot: CPU, memoria y
          // disco se fijan en el snapshot mlib-dind (ver docs/despliegue.md).
          sandbox = prepareDaytona()
          echo "Sandbox Daytona: ${sandbox}"
        }
      }
    }

    // Un componente detrás de otro: el sandbox de Daytona tiene como mucho
    // 10 GB de disco y no da para tres builds a la vez. Dentro de cada
    // componente, buildMultiArchImage construye en paralelo linux/arm64 en
    // srv01 y linux/amd64 en el sandbox, y publica el manifiesto en ghcr.io
    // (:latest y :<commit>).
    stage('Construir y subir imágenes') {
      steps {
        script {
          for (String nombre : IMAGENES.keySet()) {
            def cfg = IMAGENES[nombre]
            stage(nombre) {
              def refs = buildMultiArchImage(
                image: cfg.image,
                context: cfg.context,
                dockerfile: cfg.dockerfile,
                tags: ['latest'],
                credentialsId: env.REGISTRO_CREDENCIAL,
                agents: ['linux/arm64': 'srv01'],
                mode: 'sandboxing',
                sandboxId: sandbox
              )
              echo "${nombre}: ${refs.join(', ')}"
            }
          }
        }
      }
    }

    // Las imágenes ya están en ghcr.io: Trivy las lee del registro.
    // --ignore-unfixed deja fuera lo que aún no tiene parche; --exit-code 1
    // convierte cualquier hallazgo restante (todas las severidades) en fallo.
    stage('Trivy (paralelo)') {
      agent { label 'srv01' }
      steps {
        // Se descarga la base de datos una vez y las tres ramas la comparten:
        // Trivy la abre en solo lectura, así que admite varios procesos.
        sh '''
          docker run --rm \
            -v poketeam-trivy-cache:/root/.cache/ \
            aquasec/trivy:latest image --download-db-only
        '''
        script {
          // --cache-backend memory: la caché de escaneo en disco (fanal.db) se
          // abre en exclusiva y, con las tres ramas en el mismo volumen, las
          // que no la consiguen en 5 s mueren con «cache may be in use by
          // another process». Tampoco sirve de nada: el volumen se borra al
          // terminar la stage.
          def escanear = { String imagen ->
            withCredentials([usernamePassword(credentialsId: env.REGISTRO_CREDENCIAL,
                                              usernameVariable: 'TRIVY_USERNAME',
                                              passwordVariable: 'TRIVY_PASSWORD')]) {
              sh(
                script: 'docker run --rm ' +
                  '-e TRIVY_USERNAME -e TRIVY_PASSWORD ' +
                  '-v poketeam-trivy-cache:/root/.cache/ ' +
                  'aquasec/trivy:latest image ' +
                  '--image-src remote ' +
                  '--platform linux/amd64 ' +
                  '--scanners vuln ' +
                  '--severity UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL ' +
                  '--ignore-unfixed ' +
                  '--skip-db-update ' +
                  '--cache-backend memory ' +
                  '--timeout 15m ' +
                  '--exit-code 1 ' +
                  "${imagen}:latest",
                label: "Trivy ${imagen}"
              )
            }
          }
          def ramas = [:]
          for (String n : IMAGENES.keySet()) {
            def imagen = IMAGENES[n].image
            ramas[n] = { escanear(imagen) }
          }
          parallel(ramas)
        }
      }
      post {
        always {
          sh 'docker volume rm -f poketeam-trivy-cache 2>/dev/null || true'
          cleanDocker()
          cleanWs()
        }
      }
    }
  }

  post {
    always {
      // El sandbox cuesta dinero mientras exista: se borra siempre.
      // prepareDaytona.delete no hace fallar el build si no puede borrarlo.
      node('srv01') {
        script {
          if (sandbox) {
            prepareDaytona.delete(sandbox)
          }
        }
      }
    }
  }
}
