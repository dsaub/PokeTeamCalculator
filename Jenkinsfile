@Library('mlib') _
pipeline {
  agent any

  environment {
    API_IMAGE    = 'ghcr.io/dsaub/poketeamcalculator:latest'
    WORKER_IMAGE = 'ghcr.io/dsaub/poketeamcalculator-worker:latest'
    WEB_IMAGE    = 'ghcr.io/dsaub/poketeamcalculator-web:latest'
  }

  stages {
    stage('SCM') {
      steps {
        checkout scm
      }
    }

    // Los tres componentes se analizan a la vez. Cada rama usa su propio
    // sonar-project.properties y su propio Dockerfile.
    stage('Análisis de seguridad (paralelo)') {
      steps {
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
    }

    stage('Preparar emulación x86_64') {
      steps {
        // Registra QEMU en binfmt_misc para poder ejecutar binarios amd64 en el agente ARM
        sh 'docker run --privileged --rm tonistiigi/binfmt:latest --install amd64'
      }
    }

    stage('Build docker images (paralelo)') {
      steps {
        script {
          parallel(
            api: {
              sh '''
                docker buildx build \
                  --platform linux/amd64,linux/arm64 \
                  --load \
                  -t "$API_IMAGE" \
                  .
              '''
            },
            worker: {
              sh '''
                docker buildx build \
                  --platform linux/amd64,linux/arm64 \
                  --load \
                  -f worker/Dockerfile \
                  -t "$WORKER_IMAGE" \
                  ./worker
              '''
            },
            web: {
              sh '''
                docker buildx build \
                  --platform linux/amd64,linux/arm64 \
                  --load \
                  -f web/Dockerfile \
                  -t "$WEB_IMAGE" \
                  ./web
              '''
            }
          )
        }
      }
    }

    stage('Preparar caché de Trivy') {
      steps {
        // Se descarga la base de datos una vez y las tres ramas la comparten.
        // Se usa un volumen nombrado para poder borrarlo al terminar sin
        // depender de los permisos del usuario de Jenkins sobre /tmp.
        sh '''
          docker run --rm \
            -v poketeam-trivy-cache:/root/.cache/ \
            aquasec/trivy:latest image --download-db-only
        '''
      }
    }

    // Falla si hay cualquier vulnerabilidad con versión corregida posterior.
    // --ignore-unfixed deja fuera las que todavía no tienen parche; --exit-code 1
    // convierte cualquier hallazgo restante (todas las severidades) en fallo.
    stage('Trivy (paralelo)') {
      steps {
        script {
          def escanear = { String imagen ->
            sh(
              script: "docker run --rm " +
                "-v /var/run/docker.sock:/var/run/docker.sock " +
                "-v poketeam-trivy-cache:/root/.cache/ " +
                "aquasec/trivy:latest image " +
                "--scanners vuln " +
                "--severity UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL " +
                "--ignore-unfixed " +
                "--skip-db-update " +
                "--timeout 15m " +
                "--exit-code 1 " +
                imagen,
              label: "Trivy ${imagen}"
            )
          }
          parallel(
            api: {
              escanear(env.API_IMAGE)
            },
            worker: {
              escanear(env.WORKER_IMAGE)
            },
            web: {
              escanear(env.WEB_IMAGE)
            }
          )
        }
      }
    }

    stage('Subir imágenes') {
      steps {
        // En serie: dockerPushGhcr hace login/logout y no debe solaparse.
        dockerPushGhcr(env.API_IMAGE)
        dockerPushGhcr(env.WORKER_IMAGE)
        dockerPushGhcr(env.WEB_IMAGE)
      }
    }
  }

  post {
    always {
      script {
        // Limpieza general al terminar, tanto si el build pasa como si falla.
        // No se tocan volúmenes de datos ajenos: solo el volumen de caché de
        // Trivy de este pipeline.
        sh '''
          set +e
          echo "=== Docker antes de la limpieza ==="
          docker system df

          # Imágenes construidas por este pipeline.
          docker image rm -f "$API_IMAGE" "$WORKER_IMAGE" "$WEB_IMAGE" 2>/dev/null

          # Imágenes sin usar (incluidas base/tool antiguas) y caché de BuildKit.
          docker image prune -a -f --filter "until=24h" 2>/dev/null || true
          docker buildx prune -a -f 2>/dev/null || true

          # Caché de Trivy de este pipeline.
          docker volume rm -f poketeam-trivy-cache 2>/dev/null || true

          # Restos de versiones anteriores que usaban /tmp/trivy-cache.
          docker run --rm -v /tmp:/tmp alpine sh -c 'rm -rf /tmp/trivy-cache' 2>/dev/null || true

          echo "=== Docker después de la limpieza ==="
          docker system df || true
        '''
      }
      cleanWs()
    }
  }
}
