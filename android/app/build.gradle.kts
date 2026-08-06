plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
    alias(libs.plugins.ksp)
    alias(libs.plugins.hilt)
}

/**
 * Build configuration is read from Gradle properties rather than hardcoded, so a
 * debug build pointed at a laptop and a release build pointed at production are
 * the same source with different flags:
 *
 *   ./gradlew assembleRelease -PloadifyApiUrl=https://api.loadify.uz/api/v1
 */
fun prop(name: String, fallback: String): String =
    (project.findProperty(name) as String?) ?: fallback

android {
    namespace = "uz.loadify"
    compileSdk = 36

    defaultConfig {
        applicationId = "uz.loadify"
        minSdk = 24
        targetSdk = 36
        versionCode = 1
        versionName = "1.0.0"

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    // Ships only the three languages the product actually has. Without this the
    // APK carries every locale AndroidX translates into, which is dead weight
    // on the low-end devices this market runs on.
    androidResources {
        localeFilters += listOf("uz", "ru", "en")
    }

    buildTypes {
        debug {
            // 10.0.2.2 is the emulator's alias for the host machine. On a
            // physical device pass your LAN address instead — localhost there
            // is the phone.
            buildConfigField(
                "String",
                "API_URL",
                "\"${prop("loadifyApiUrl", "http://10.0.2.2:8080/api/v1")}\"",
            )
            buildConfigField(
                "String",
                "WS_URL",
                "\"${prop("loadifyWsUrl", "ws://10.0.2.2:8080/api/v1/ws")}\"",
            )
        }
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro",
            )
            buildConfigField(
                "String",
                "API_URL",
                "\"${prop("loadifyApiUrl", "https://api.loadify.uz/api/v1")}\"",
            )
            buildConfigField(
                "String",
                "WS_URL",
                "\"${prop("loadifyWsUrl", "wss://api.loadify.uz/api/v1/ws")}\"",
            )
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
        // java.time is API 26+, and minSdk is 24. Without desugaring the date
        // formatters crash on Android 7 — a real slice of this market's
        // devices, and exactly the users least likely to report it.
        isCoreLibraryDesugaringEnabled = true
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    packaging {
        resources.excludes += "/META-INF/{AL2.0,LGPL2.1}"
    }
}

kotlin {
    compilerOptions {
        jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17)
        // Warnings in a codebase this size are signal, not noise; letting them
        // accumulate is how the one that mattered gets lost.
        allWarningsAsErrors.set(true)
    }
}

dependencies {
    coreLibraryDesugaring(libs.desugar.jdk.libs)

    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.appcompat)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.androidx.activity.compose)

    implementation(platform(libs.compose.bom))
    implementation(libs.compose.ui)
    implementation(libs.compose.ui.graphics)
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.compose.material3)
    implementation(libs.compose.material.icons.extended)
    debugImplementation(libs.compose.ui.tooling)

    implementation(libs.androidx.navigation.compose)

    implementation(libs.retrofit)
    implementation(libs.retrofit.serialization)
    implementation(libs.okhttp)
    implementation(libs.kotlinx.serialization.json)
    debugImplementation(libs.okhttp.logging)

    implementation(libs.androidx.datastore.preferences)
    implementation(libs.androidx.security.crypto)
    implementation(libs.play.services.location)

    implementation(libs.hilt.android)
    implementation(libs.hilt.navigation.compose)
    ksp(libs.hilt.compiler)

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.okhttp.mockwebserver)
    testImplementation(libs.kotlinx.serialization.json)
}
