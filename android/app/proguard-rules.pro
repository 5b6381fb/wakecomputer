# Retrofit / kotlinx.serialization
-keepattributes Signature, InnerClasses, EnclosingMethod
-keepattributes RuntimeVisibleAnnotations, RuntimeVisibleParameterAnnotations
-keepclassmembers,allowshrinking,allowobfuscation interface * {
    @retrofit2.http.* <methods>;
}
-dontwarn okhttp3.**
-dontwarn okio.**
-dontwarn retrofit2.**
-dontwarn org.jetbrains.annotations.**

# Keep serializable DTOs
-keep,includedescriptorclasses class com.wakecomputer.data.**$$serializer { *; }
-keepclassmembers class com.wakecomputer.data.** {
    *** Companion;
}
-keepclasseswithmembers class com.wakecomputer.data.** {
    kotlinx.serialization.KSerializer serializer(...);
}
