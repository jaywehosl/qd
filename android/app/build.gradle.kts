plugins {
    id("com.android.application")
}

android {
    namespace = "ru.quicdiver.client"
    compileSdk = 35

    signingConfigs {
        create("release") {
            val store = rootProject.file("qd-release.jks")
            if (store.exists()) {
                storeFile = store
                storePassword = System.getenv("QD_KEYSTORE_PASS") ?: ""
                keyAlias = "qd"
                keyPassword = System.getenv("QD_KEYSTORE_PASS") ?: ""
            }
        }
    }

    defaultConfig {
        applicationId = "ru.quicdiver.client"
        minSdk = 33
        targetSdk = 35
        versionCode = 3
        versionName = "0.0.2"
        ndk {
            abiFilters += "arm64-v8a"
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    buildTypes {
        debug {
            isDebuggable = true
        }
        release {
            if (rootProject.file("qd-release.jks").exists()) {
                signingConfig = signingConfigs.getByName("release")
            }
            isDebuggable = false
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
        }
    }

    packaging {
        jniLibs {
            useLegacyPackaging = false
        }
    }
}

dependencies {
    implementation(files("libs/qdmobile.aar"))
}
