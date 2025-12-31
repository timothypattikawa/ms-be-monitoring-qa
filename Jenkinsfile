pipeline {
    agent any

    stages {
        stage('Checkout Code') {
            steps {
                // Jenkins otomatis melakukan checkout, tapi kita bisa tambahkan echo
                echo 'Mengambil source code dari GitHub...'
            }
        }
        
        stage('Build') {
            steps {
                echo 'Sedang melakukan proses Build...'
                // Contoh command simple:
                sh 'ls -la' 
            }
        }

        stage('Test') {
            steps {
                echo 'Sedang menjalankan Unit Test...'
            }
        }
    }
}