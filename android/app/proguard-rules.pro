# kotlinx.serialization generates a companion .serializer() per @Serializable
# class and looks it up reflectively. R8 cannot see that reference, so without
# these rules a release build strips the serializers and every API response
# fails to parse — a failure that only appears in release, never in debug.
-keepattributes *Annotation*, InnerClasses
-dontnote kotlinx.serialization.**

-keepclassmembers class kotlinx.serialization.json.** {
    *** Companion;
}
-keepclasseswithmembers class kotlinx.serialization.json.** {
    kotlinx.serialization.KSerializer serializer(...);
}

-keep,includedescriptorclasses class uz.loadify.**$$serializer { *; }
-keepclassmembers class uz.loadify.** {
    *** Companion;
}
-keepclasseswithmembers class uz.loadify.** {
    kotlinx.serialization.KSerializer serializer(...);
}

# Retrofit reads generic signatures off interface methods at runtime to pick a
# converter; erasing them turns every call into a raw-type failure.
-keepattributes Signature, Exceptions
-keep,allowobfuscation interface retrofit2.Call
-keep,allowobfuscation class retrofit2.Response

# OkHttp ships references to optional platform providers that are absent on
# Android. These are warnings about code that never runs here.
-dontwarn okhttp3.internal.platform.**
-dontwarn org.conscrypt.**
-dontwarn org.bouncycastle.**
-dontwarn org.openjsse.**
