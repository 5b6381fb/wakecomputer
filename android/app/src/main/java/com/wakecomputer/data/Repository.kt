package com.wakecomputer.data

import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import retrofit2.HttpException
import java.io.IOException

sealed class AppError(message: String) : Exception(message) {
    class Network(message: String) : AppError(message)
    class Unauthorized(message: String) : AppError(message)
    class Server(message: String) : AppError(message)
}

class Repository(
    private val apiProvider: ApiProvider,
    private val store: LocalStore
) {
    private val refreshMutex = Mutex()

    suspend fun serverUrl(): String? = store.server()

    suspend fun login(serverInput: String, username: String, password: String): UserDto {
        val server = normalizeServer(serverInput)
        val response = try {
            apiProvider.api(server).login(LoginRequest(username.trim(), password))
        } catch (e: HttpException) {
            throw mapHttp(e)
        } catch (e: IOException) {
            throw AppError.Network("无法连接服务器：${e.message ?: "网络错误"}")
        }
        AuthState.accessToken = response.accessToken
        store.saveSession(server, response.refreshToken)
        return response.user
    }

    /**
     * Called on app start. A stored refresh token is only discarded when the
     * server explicitly rejects it. A timeout, DNS failure or captive portal
     * must not log the user out: the app opens with cached data and retries
     * once the server is reachable again.
     */
    suspend fun restoreSession(): Boolean {
        val server = store.server() ?: return false
        val refresh = store.refreshToken() ?: return false
        return try {
            val res = apiProvider.api(server).refresh(RefreshRequest(refresh))
            AuthState.accessToken = res.accessToken
            store.saveSession(server, res.refreshToken)
            true
        } catch (e: HttpException) {
            if (e.code() == 401 || e.code() == 403) {
                AuthState.accessToken = null
                store.clearSession()
                false
            } else {
                // Server is reachable but unhealthy; keeping the session is safer
                // than forcing a password re-entry over a temporary 5xx.
                true
            }
        } catch (e: Exception) {
            true
        }
    }

    suspend fun logout() {
        AuthState.accessToken = null
        store.clearSession()
    }

    /** Login form values kept for the next launch. */
    suspend fun savedCredentials(): Pair<String, String>? = store.savedCredentials()

    suspend fun rememberCredentials(username: String, password: String) =
        store.rememberCredentials(username, password)

    suspend fun forgetCredentials() = store.forgetCredentials()

    suspend fun <T> authed(block: suspend (WakeApi) -> T): T {
        val server = store.server() ?: throw AppError.Unauthorized("请先登录")
        val api = apiProvider.api(server)
        return try {
            block(api)
        } catch (e: HttpException) {
            if (e.code() != 401) throw mapHttp(e)
            when (refresh()) {
                RefreshOutcome.OK -> block(api)
                RefreshOutcome.EXPIRED -> throw AppError.Unauthorized("登录已过期，请重新登录")
                RefreshOutcome.UNAVAILABLE ->
                    throw AppError.Network("暂时连不上服务器，登录状态已保留，网络恢复后自动重试")
            }
        } catch (e: IOException) {
            throw AppError.Network("网络请求失败：${e.message ?: "请检查网络"}")
        }
    }

    private enum class RefreshOutcome { OK, EXPIRED, UNAVAILABLE }

    private suspend fun refresh(): RefreshOutcome = refreshMutex.withLock {
        val server = store.server() ?: return RefreshOutcome.EXPIRED
        val token = store.refreshToken() ?: return RefreshOutcome.EXPIRED
        try {
            val res = apiProvider.api(server).refresh(RefreshRequest(token))
            AuthState.accessToken = res.accessToken
            store.saveSession(server, res.refreshToken)
            RefreshOutcome.OK
        } catch (e: HttpException) {
            if (e.code() == 401 || e.code() == 403) {
                AuthState.accessToken = null
                store.clearSession()
                RefreshOutcome.EXPIRED
            } else {
                RefreshOutcome.UNAVAILABLE
            }
        } catch (e: Exception) {
            // Keep the refresh token: the server was simply unreachable.
            RefreshOutcome.UNAVAILABLE
        }
    }

    private fun normalizeServer(input: String): String {
        var s = input.trim().trimEnd('/')
        if (s.isEmpty()) throw AppError.Network("请填写服务器地址")
        if (!s.startsWith("http://") && !s.startsWith("https://")) {
            s = "https://$s"
        }
        return s
    }

    private fun mapHttp(e: HttpException): AppError {
        val raw = runCatching { e.response()?.errorBody()?.string() }.getOrNull()
        val message = runCatching {
            apiProvider.json.parseToJsonElement(raw ?: "{}")
                .jsonObject["error"]?.jsonObject?.get("message")?.jsonPrimitive?.contentOrNull
        }.getOrNull()
        return if (e.code() == 401) {
            AppError.Unauthorized(message ?: "登录已过期，请重新登录")
        } else {
            AppError.Server(message ?: "请求失败（HTTP ${e.code()}）")
        }
    }

    // ---------- auth ----------

    suspend fun me(): MeResponse = authed { it.me() }

    suspend fun changePassword(old: String, new: String) =
        authed { it.changePassword(ChangePasswordRequest(old, new)) }

    // ---------- hosts ----------

    suspend fun hosts(): List<HostDto> {
        val list = authed { it.hosts().items }
        runCatching { store.saveHosts(apiProvider.json.encodeToString(list)) }
        return list
    }

    suspend fun discover(subnet: String? = null): DiscoveryResponse =
        authed { it.discover(subnet) }

    suspend fun cachedHosts(): List<HostDto> = runCatching {
        store.hostsJson()?.let { apiProvider.json.decodeFromString<List<HostDto>>(it) } ?: emptyList()
    }.getOrDefault(emptyList())

    suspend fun host(id: String): HostDto = authed { it.host(id) }

    suspend fun createHost(payload: HostPayload): HostDto = authed { it.createHost(payload) }

    suspend fun updateHost(id: String, payload: HostPayload): HostDto =
        authed { it.updateHost(id, payload) }

    suspend fun deleteHost(id: String) = authed { it.deleteHost(id) }

    suspend fun wake(id: String): WakeResponse = authed { it.wake(id) }

    suspend fun power(id: String, action: String, delaySeconds: Int = 30): PowerResponse =
        authed { it.power(id, PowerRequest(action, delaySeconds)) }

    suspend fun enrollCode(id: String): EnrollCodeResponse = authed { it.enrollCode(id) }

    suspend fun removeAgent(id: String) = authed { it.removeAgent(id) }

    // ---------- groups ----------

    suspend fun groups(): List<GroupDto> = authed { it.groups().items }

    suspend fun createGroup(name: String): GroupDto = authed { it.createGroup(GroupPayload(name)) }

    suspend fun deleteGroup(id: String) = authed { it.deleteGroup(id) }

    // ---------- batch ----------

    suspend fun batch(hostIds: List<String>, action: String, delaySeconds: Int = 0): BatchResponse =
        authed { it.batch(BatchRequest(hostIds, action, delaySeconds)) }

    suspend fun job(id: String): JobDto = authed { it.job(id) }

    suspend fun cancelJob(id: String): JobDto = authed { it.cancelJob(id) }

    // ---------- audit / settings ----------

    suspend fun audit(hostId: String? = null, cursor: Long? = null): AuditResponse =
        authed { it.audit(limit = 50, cursor = cursor, hostId = hostId) }

    suspend fun settings(): SettingsDto = authed { it.settings() }

    suspend fun updateSettings(body: SettingsDto): SettingsDto = authed { it.updateSettings(body) }
}
