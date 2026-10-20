package com.wakecomputer.ui.detail

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.wakecomputer.data.AppError
import com.wakecomputer.data.AuditDto
import com.wakecomputer.data.EnrollCodeResponse
import com.wakecomputer.data.HostDto
import com.wakecomputer.data.Repository
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

class HostDetailViewModel(
    private val repo: Repository,
    private val hostId: String
) : ViewModel() {
    var host by mutableStateOf<HostDto?>(null)
        private set
    var audit by mutableStateOf<List<AuditDto>>(emptyList())
        private set
    var loading by mutableStateOf(true)
        private set
    var error by mutableStateOf<String?>(null)
        private set
    var message by mutableStateOf<String?>(null)
    var busy by mutableStateOf(false)
        private set
    var enroll by mutableStateOf<EnrollCodeResponse?>(null)
        private set

    private var pollJob: Job? = null

    fun startPolling() {
        if (pollJob != null) return
        pollJob = viewModelScope.launch {
            load(silent = false)
            while (isActive) {
                delay(5000)
                load(silent = true)
            }
        }
    }

    fun stopPolling() {
        pollJob?.cancel()
        pollJob = null
    }

    fun refresh() {
        viewModelScope.launch { load(silent = false) }
    }

    private suspend fun load(silent: Boolean) {
        if (!silent) loading = true
        try {
            host = repo.host(hostId)
            audit = repo.audit(hostId = hostId).items.take(8)
            error = null
        } catch (e: AppError) {
            if (host == null) error = e.message else message = e.message
        } catch (e: Exception) {
            if (host == null) error = e.message ?: "加载失败" else message = e.message
        } finally {
            loading = false
        }
    }

    fun wake() {
        if (busy) return
        busy = true
        viewModelScope.launch {
            try {
                val res = repo.wake(hostId)
                message = if (res.accepted) "唤醒指令已发送，正在等待主机上线" else "唤醒指令发送失败"
                host = repo.host(hostId)
            } catch (e: Exception) {
                message = e.message ?: "唤醒失败"
            } finally {
                busy = false
            }
        }
    }

    fun power(action: String, delaySeconds: Int = 30) {
        if (busy) return
        busy = true
        viewModelScope.launch {
            try {
                repo.power(hostId, action, delaySeconds)
                message = when (action) {
                    "shutdown" -> "关机指令已下发，${delaySeconds} 秒后执行"
                    "reboot" -> "重启指令已下发，${delaySeconds} 秒后执行"
                    else -> "已取消待执行的关机"
                }
            } catch (e: Exception) {
                message = e.message ?: "指令下发失败"
            } finally {
                busy = false
            }
        }
    }

    fun requestEnrollCode() {
        if (busy) return
        busy = true
        viewModelScope.launch {
            try {
                enroll = repo.enrollCode(hostId)
            } catch (e: Exception) {
                message = e.message ?: "生成接入码失败"
            } finally {
                busy = false
            }
        }
    }

    fun dismissEnroll() {
        enroll = null
    }

    fun removeAgent() {
        if (busy) return
        busy = true
        viewModelScope.launch {
            try {
                repo.removeAgent(hostId)
                message = "已解绑 Agent"
                host = repo.host(hostId)
            } catch (e: Exception) {
                message = e.message ?: "解绑失败"
            } finally {
                busy = false
            }
        }
    }

    fun delete(onDeleted: () -> Unit) {
        if (busy) return
        busy = true
        viewModelScope.launch {
            try {
                repo.deleteHost(hostId)
                onDeleted()
            } catch (e: Exception) {
                message = e.message ?: "删除失败"
                busy = false
            }
        }
    }

    fun consumeMessage() {
        message = null
    }
}
