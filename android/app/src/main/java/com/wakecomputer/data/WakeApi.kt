package com.wakecomputer.data

import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.PATCH
import retrofit2.http.POST
import retrofit2.http.PUT
import retrofit2.http.Path
import retrofit2.http.Query

interface WakeApi {
    @POST("api/v1/auth/login")
    suspend fun login(@Body body: LoginRequest): TokenResponse

    @POST("api/v1/auth/refresh")
    suspend fun refresh(@Body body: RefreshRequest): TokenResponse

    @GET("api/v1/auth/me")
    suspend fun me(): MeResponse

    @POST("api/v1/auth/password")
    suspend fun changePassword(@Body body: ChangePasswordRequest)

    @GET("api/v1/hosts")
    suspend fun hosts(): HostsResponse

    @GET("api/v1/discovery")
    suspend fun discover(
        @Query("subnet") subnet: String? = null,
        @Query("scan") scan: String = "1"
    ): DiscoveryResponse

    @GET("api/v1/hosts/{id}")
    suspend fun host(@Path("id") id: String): HostDto

    @POST("api/v1/hosts")
    suspend fun createHost(@Body body: HostPayload): HostDto

    @PATCH("api/v1/hosts/{id}")
    suspend fun updateHost(@Path("id") id: String, @Body body: HostPayload): HostDto

    @DELETE("api/v1/hosts/{id}")
    suspend fun deleteHost(@Path("id") id: String)

    @POST("api/v1/hosts/{id}/wake")
    suspend fun wake(@Path("id") id: String): WakeResponse

    @POST("api/v1/hosts/{id}/power")
    suspend fun power(@Path("id") id: String, @Body body: PowerRequest): PowerResponse

    @POST("api/v1/hosts/{id}/agent/enroll-code")
    suspend fun enrollCode(@Path("id") id: String): EnrollCodeResponse

    @DELETE("api/v1/hosts/{id}/agent")
    suspend fun removeAgent(@Path("id") id: String)

    @GET("api/v1/groups")
    suspend fun groups(): GroupsResponse

    @POST("api/v1/groups")
    suspend fun createGroup(@Body body: GroupPayload): GroupDto

    @DELETE("api/v1/groups/{id}")
    suspend fun deleteGroup(@Path("id") id: String)

    @POST("api/v1/batch")
    suspend fun batch(@Body body: BatchRequest): BatchResponse

    @GET("api/v1/jobs/{id}")
    suspend fun job(@Path("id") id: String): JobDto

    @POST("api/v1/jobs/{id}/cancel")
    suspend fun cancelJob(@Path("id") id: String): JobDto

    @GET("api/v1/audit")
    suspend fun audit(
        @Query("limit") limit: Int = 50,
        @Query("cursor") cursor: Long? = null,
        @Query("hostId") hostId: String? = null
    ): AuditResponse

    @GET("api/v1/settings")
    suspend fun settings(): SettingsDto

    @PUT("api/v1/settings")
    suspend fun updateSettings(@Body body: SettingsDto): SettingsDto
}

@kotlinx.serialization.Serializable
data class ChangePasswordRequest(val oldPassword: String, val newPassword: String)
