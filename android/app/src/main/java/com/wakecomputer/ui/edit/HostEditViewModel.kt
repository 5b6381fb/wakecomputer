package com.wakecomputer.ui.edit

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.wakecomputer.data.DiscoveryDeviceDto
import com.wakecomputer.data.GroupDto
import com.wakecomputer.data.HostPayload
import com.wakecomputer.data.Repository
import kotlinx.coroutines.launch

class HostEditViewModel(
    private val repo: Repository,
    private val hostId: String?
) : ViewModel() {
    val isNew: Boolean = hostId == null || hostId == "new"

    var name by mutableStateOf("")
    var mac by mutableStateOf("")
    var ip by mutableStateOf("")
    var osType by mutableStateOf("windows")
    var broadcast by mutableStateOf("192.168.1.255")
    var wolPort by mutableStateOf("9")
    var probePort by mutableStateOf("")
    var notes by mutableStateOf("")
    var groups by mutableStateOf<List<GroupDto>>(emptyList())
    var selectedGroups by mutableStateOf<Set<String>>(emptySet())
    var loading by mutableStateOf(!isNew)
    var saving by mutableStateOf(false)
    var error by mutableStateOf<String?>(null)
    var showDiscovery by mutableStateOf(false)
        private set
    var discovering by mutableStateOf(false)
        private set
    var discovered by mutableStateOf<List<DiscoveryDeviceDto>>(emptyList())
        private set
    var discoveryError by mutableStateOf<String?>(null)
        private set

    init {
        viewModelScope.launch {
            try {
                groups = repo.groups()
                if (!isNew && hostId != null) {
                    val host = repo.host(hostId)
                    name = host.name
                    mac = host.mac
                    ip = host.ip
                    osType = host.osType
                    broadcast = host.broadcastTargets.joinToString(", ")
                    wolPort = host.wolPort.toString()
                    probePort = if (host.probePort > 0) host.probePort.toString() else ""
                    notes = host.notes
                    selectedGroups = host.groups.map { it.id }.toSet()
                }
            } catch (e: Exception) {
                error = e.message ?: "加载失败"
            } finally {
                loading = false
            }
        }
    }

    fun toggleGroup(id: String) {
        selectedGroups = if (id in selectedGroups) selectedGroups - id else selectedGroups + id
    }

    fun discover() {
        if (discovering) return
        showDiscovery = true
        discovering = true
        discoveryError = null
        viewModelScope.launch {
            try {
                discovered = repo.discover().items
            } catch (e: Exception) {
                discoveryError = e.message ?: "扫描局域网失败"
            } finally {
                discovering = false
            }
        }
    }

    fun dismissDiscovery() {
        showDiscovery = false
    }

    fun applyDevice(device: DiscoveryDeviceDto) {
        if (device.added) return
        name = device.hostname.ifBlank { "设备-${device.ip.substringAfterLast('.', device.ip)}" }
        mac = device.mac
        ip = device.ip
        broadcast = broadcastFor(device.ip)
        error = null
        showDiscovery = false
    }

    private fun broadcastFor(ip: String): String {
        val parts = ip.split(".")
        return if (parts.size == 4) "${parts[0]}.${parts[1]}.${parts[2]}.255" else broadcast
    }

    fun save(onSaved: (String) -> Unit) {
        val trimmedName = name.trim()
        if (trimmedName.isEmpty()) {
            error = "请填写主机名称"
            return
        }
        if (!MAC_REGEX.matches(mac.trim())) {
            error = "MAC 地址格式不正确，例如 AA:BB:CC:DD:EE:FF"
            return
        }
        val wol = wolPort.toIntOrNull() ?: 9
        if (wol !in 1..65535) {
            error = "WoL 端口需在 1-65535 之间"
            return
        }
        val probe = if (probePort.isBlank()) 0 else probePort.toIntOrNull() ?: -1
        if (probe !in 0..65535) {
            error = "探测端口需在 0-65535 之间"
            return
        }
        if (ip.isNotBlank() && !IP_REGEX.matches(ip.trim())) {
            error = "IP 地址格式不正确"
            return
        }

        val targets = broadcast.split(',', ' ', '\n')
            .map { it.trim() }
            .filter { it.isNotEmpty() }

        val payload = HostPayload(
            name = trimmedName,
            osType = osType,
            mac = mac.trim(),
            ip = ip.trim(),
            broadcastTargets = targets,
            wolPort = wol,
            probePort = probe,
            notes = notes.trim(),
            groupIds = selectedGroups.toList()
        )

        if (saving) return
        saving = true
        viewModelScope.launch {
            try {
                val saved = if (isNew) {
                    repo.createHost(payload)
                } else {
                    repo.updateHost(hostId!!, payload)
                }
                onSaved(saved.id)
            } catch (e: Exception) {
                error = e.message ?: "保存失败"
                saving = false
            }
        }
    }

    companion object {
        private val MAC_REGEX = Regex("^([0-9A-Fa-f]{2}[:-]){5}[0-9A-Fa-f]{2}$")
        private val IP_REGEX = Regex("^(\\d{1,3}\\.){3}\\d{1,3}$")
    }
}
