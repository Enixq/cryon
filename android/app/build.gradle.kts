plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

val releaseVersion = (System.getenv("CRYON_RELEASE_VERSION") ?: "v0.2.15").removePrefix("v")
val versionParts = releaseVersion.split(".").map { it.toInt() }

android {
    namespace = "ru.cryon.app"
    compileSdk = 36

    defaultConfig {
        applicationId = "ru.cryon.app"
        minSdk = 24
        targetSdk = 36
        versionCode = versionParts[0] * 1000000 + versionParts[1] * 1000 + versionParts[2]
        versionName = releaseVersion
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    signingConfigs {
        create("cryonRelease") {
            val keystorePath = System.getenv("CRYON_KEYSTORE_PATH")
            if (!keystorePath.isNullOrBlank()) {
                storeFile = file(keystorePath)
                storePassword = System.getenv("CRYON_KEYSTORE_PASSWORD")
                keyAlias = System.getenv("CRYON_KEY_ALIAS")
                keyPassword = System.getenv("CRYON_KEY_PASSWORD")
            }
        }
    }

    buildTypes {
        debug {
            if (System.getenv("CRYON_SEPARATE_TEST_APP") == "1") {
                applicationIdSuffix = ".yandextest"
                versionNameSuffix = "-yandextest"
            }
        }
        release {
            isMinifyEnabled = false
            signingConfig = signingConfigs.getByName("cryonRelease")
        }
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlin {
        compilerOptions {
            jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17)
        }
    }
    packaging {
        jniLibs {
            useLegacyPackaging = false
        }
    }
}

dependencies {
    // Go-Р±СЌРєРµРЅРґ (gomobile bind): mobile.Start(filesDir) / mobile.Stop()
    implementation(files("libs/cryonmobile.aar"))
    implementation("androidx.core:core-ktx:1.15.0")
    implementation("androidx.appcompat:appcompat:1.7.0")
    implementation("androidx.webkit:webkit:1.12.1")
    implementation("androidx.browser:browser:1.8.0")
    implementation("androidx.documentfile:documentfile:1.0.1")
    implementation("androidx.media:media:1.7.0")
    implementation("androidx.media3:media3-exoplayer:1.8.0")
    implementation("androidx.media3:media3-exoplayer-hls:1.8.0")
    implementation("androidx.media3:media3-session:1.8.0")
    implementation("androidx.media3:media3-datasource:1.8.0")
    implementation("androidx.media3:media3-database:1.8.0")
    androidTestImplementation("androidx.test:runner:1.6.2")
    androidTestImplementation("androidx.test.ext:junit:1.2.1")
}
