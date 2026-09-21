pipeline {
    agent any

    environment {
        // Ganti nama-image sesuai keinginan
        IMAGE_NAME = "ms-monitoring-qa-be"
        DOCKER_TAG = "${BUILD_NUMBER}"
    }

    stages {
        stage('Build Docker Image') {
            steps {
                script {
                    echo 'Membangun Docker Image...'
                    // Pastikan ada file 'Dockerfile' di repo Anda
                    sh "docker build -t ${IMAGE_NAME}:${DOCKER_TAG} ."
                }
            }
        }

        stage('Test Image') {
            steps {
                echo 'Testing container...'
                // Contoh simple: Cek versi (sesuaikan dengan bahasa prog, misal node -v)
                sh "docker run --rm ${IMAGE_NAME}:${DOCKER_TAG} echo 'Container Berjalan!'"
            }
        }
        
        // Uncomment tahap ini jika nanti sudah siap push ke AWS ECR / DockerHub
        /*
        stage('Push to Registry') {
            steps {
                echo 'Pushing image...'
            }
        }
        */
    }
    
    post {
        always {
            // Bersihkan image agar disk server tidak penuh
            sh "docker rmi ${IMAGE_NAME}:${DOCKER_TAG} || true"
        }
    }
}
