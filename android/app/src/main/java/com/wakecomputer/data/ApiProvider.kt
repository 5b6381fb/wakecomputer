package com.wakecomputer.data

import android.os.Build
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import retrofit2.Retrofit
import retrofit2.converter.kotlinx.serialization.asConverterFactory
import java.util.concurrent.TimeUnit

object AuthState {
    @Volatile
    var accessToken: String? = null
}

class ApiProvider {
    val json = Json {
        ignoreUnknownKeys = true
        explicitNulls = false
        encodeDefaults = true
    }

    private val client = OkHttpClient.Builder()
        .connectTimeout(10, TimeUnit.SECONDS)
        .readTimeout(30, TimeUnit.SECONDS)
        .writeTimeout(30, TimeUnit.SECONDS)
        .addInterceptor { chain ->
            val builder = chain.request().newBuilder()
                .header("Accept", "application/json")
                .header("X-Device-Name", "${Build.MANUFACTURER} ${Build.MODEL}".trim())
            AuthState.accessToken?.let { builder.header("Authorization", "Bearer $it") }
            chain.proceed(builder.build())
        }
        .build()

    @Volatile
    private var cache: Pair<String, WakeApi>? = null

    fun api(serverUrl: String): WakeApi {
        val base = serverUrl.trimEnd('/') + "/"
        cache?.let { if (it.first == base) return it.second }
        val api = Retrofit.Builder()
            .baseUrl(base)
            .client(client)
            .addConverterFactory(json.asConverterFactory("application/json".toMediaType()))
            .build()
            .create(WakeApi::class.java)
        cache = base to api
        return api
    }
}
