package com.wakecomputer.data

import kotlinx.serialization.Serializable

@Serializable
data class LoginRequest(val username: String, val password: String)

@Serializable
data class RefreshRequest(val refreshToken: String)

@Serializable
data class UserDto(val id: String = "", val username: String = "")

@Serializable
data class TokenResponse(
    val accessToken: String = "",
    val refreshToken: String = "",
    val expiresIn: Long = 0,
    val user: UserDto = UserDto()
)

@Serializable
data class GroupDto(val id: String = "", val name: String = "", val sortOrder: Int = 0)

@Serializable
data class GroupsResponse(val items: List<GroupDto> = emptyList())

@Serializable
data class GroupPayload(val name: String, val sortOrder: Int = 0)

@Serializable
data class CapabilitiesDto(
    val wake: Boolean = true,
    val shutdown: Boolean = false,
    val reboot: Boolean = false,
    val cancel: Boolean = false
)

@Serializable
data class HostDto(
    val id: String = "",
    val name: String = "",
    val osType: String = "other",
    val mac: String = "",
    val ip: String = "",
    val broadcastTargets: List<String> = emptyList(),
    val wolPort: Int = 9,
    val probePort: Int = 0,
    val notes: String = "",
    val sortOrder: Int = 0,
    val groups: List<GroupDto> = emptyList(),
    val status: String = "unknown",
    val lastSeenAt: Long? = null,
    val agentVersion: String? = null,
    val capabilities: CapabilitiesDto = CapabilitiesDto(),
    val createdAt: Long = 0,
    val updatedAt: Long = 0
)

@Serializable
data class HostsResponse(
    val items: List<HostDto> = emptyList(),
    val serverTime: String = ""
)

@Serializable
data class HostPayload(
    val name: String,
    val osType: String,
    val mac: String,
    val ip: String = "",
    val broadcastTargets: List<String> = emptyList(),
    val wolPort: Int = 9,
    val probePort: Int = 0,
    val notes: String = "",
    val sortOrder: Int = 0,
    val groupIds: List<String> = emptyList()
)

@Serializable
data class TargetResult(
    val address: String = "",
    val ok: Boolean = false,
    val error: String? = null
)

@Serializable
data class WakeResponse(
    val accepted: Boolean = false,
    val targets: List<TargetResult> = emptyList(),
    val status: String = "waking",
    val etaSeconds: Int = 90
)

@Serializable
data class PowerRequest(val action: String, val delaySeconds: Int = 30)

@Serializable
data class PowerResponse(
    val accepted: Boolean = false,
    val executeAt: String? = null,
    val delaySeconds: Int = 0
)

@Serializable
data class EnrollCodeResponse(
    val code: String = "",
    val expiresAt: String = "",
    val linuxCommand: String = "",
    val windowsCommand: String = "",
    val agentPort: Int = 45871
)

@Serializable
data class BatchRequest(
    val hostIds: List<String>,
    val action: String,
    val delaySeconds: Int = 0
)

@Serializable
data class BatchResponse(
    val jobId: String = "",
    val job: JobDto = JobDto()
)

@Serializable
data class JobResultDto(
    val hostId: String = "",
    val name: String = "",
    val status: String = "queued",
    val error: String? = null,
    val at: Long = 0
)

@Serializable
data class JobDto(
    val id: String = "",
    val action: String = "",
    val status: String = "running",
    val total: Int = 0,
    val done: Int = 0,
    val succeeded: Int = 0,
    val failed: Int = 0,
    val canceled: Int = 0,
    val delaySeconds: Int = 0,
    val results: List<JobResultDto> = emptyList(),
    val createdAt: Long = 0,
    val finishedAt: Long? = null
)

@Serializable
data class AuditDto(
    val id: Long = 0,
    val userId: String = "",
    val username: String = "",
    val action: String = "",
    val hostId: String = "",
    val hostName: String = "",
    val result: String = "",
    val detail: String = "",
    val ip: String = "",
    val createdAt: Long = 0
)

@Serializable
data class AuditResponse(
    val items: List<AuditDto> = emptyList(),
    val nextCursor: Long? = null
)

@Serializable
data class SettingsDto(
    val wolDefaultBroadcast: String = "",
    val batchConcurrency: Int = 4,
    val auditRetentionDays: Int = 180
)

@Serializable
data class MeResponse(
    val user: UserDto = UserDto(),
    val version: String = "",
    val server: ServerInfoDto = ServerInfoDto()
)

@Serializable
data class ServerInfoDto(
    val publicUrl: String = "",
    val time: String = ""
)

@Serializable
data class DiscoveryDeviceDto(
    val ip: String = "",
    val mac: String = "",
    val hostname: String = "",
    val vendor: String = "",
    val added: Boolean = false
)

@Serializable
data class DiscoveryResponse(
    val subnet: String = "",
    val scannedAt: String = "",
    val items: List<DiscoveryDeviceDto> = emptyList()
)
