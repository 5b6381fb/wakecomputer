package com.wakecomputer.ui.hosts

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.wakecomputer.data.AppError
import com.wakecomputer.data.GroupDto
import com.wakecomputer.data.HostDto
import com.wakecomputer.data.Repository
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

class HostListViewModel(private val repo: Repository) : ViewModel() {
    var hosts by mutableStateOf<List<HostDto>>(emptyList())
        private set
    var groups by mutableStateOf<List<GroupDto>>(emptyList())
        private set
    var loading by mutableStateOf(true)
        private set
    var error by mutableStateOf<String?>(null)
        private set
    var query by mutableStateOf("")
    var groupFilter by mutableStateOf<String?>(null)
    var selected by mutableStateOf<Set<String>>(emptySet())
        private set
    var message by mutableStateOf<String?>(null)
    var pendingJobId by mutableStateOf<String?>(null)
        private set
    var busy by mutableStateOf(false)
        private set
    var fromCache by mutableStateOf(false)
        private set

    private var pollJob: Job? = null

    val selectionMode: Boolean get() = selected.isNotEmpty()

    val filtered: List<HostDto>
        get() = hosts.filter { host ->
            val matchesGroup = groupFilter == null || host.groups.any { it.id == groupFilter }
            val q = query.trim()
            val matchesQuery = q.isEmpty() ||
                host.name.contains(q, ignoreCase = true) ||
                host.ip.contains(q) ||
                host.notes.contains(q, ignoreCase = true)
            matchesGroup && matchesQuery
        }

    fun startPolling() {
        if (pollJob != null) return
        pollJob = viewModelScope.launch {
            load(silent = hosts.isNotEmpty())
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
            hosts = repo.hosts()
            groups = repo.groups()
            error = null
            fromCache = false
            val ids = hosts.map { it.id }.toSet()
            selected = selected.filter { it in ids }.toSet()
        } catch (e: AppError) {
            offline(e.message)
        } catch (e: Exception) {
            offline(e.message ?: "加载失败")
        } finally {
            loading = false
        }
    }

    private suspend fun offline(reason: String?) {
        if (hosts.isEmpty()) {
            val cached = repo.cachedHosts()
            if (cached.isNotEmpty()) {
                hosts = cached
                fromCache = true
                message = "网络不可用，正在显示缓存数据"
                return
            }
        }
        if (hosts.isEmpty()) error = reason else message = reason
    }

    fun toggle(hostId: String) {
        selected = if (hostId in selected) selected - hostId else selected + hostId
    }

    fun selectAll() {
        selected = filtered.map { it.id }.toSet()
    }

    fun clearSelection() {
        selected = emptySet()
    }

    fun runBatch(action: String, delaySeconds: Int = 0) {
        if (selected.isEmpty() || busy) return
        busy = true
        viewModelScope.launch {
            try {
                val response = repo.batch(selected.toList(), action, delaySeconds)
                pendingJobId = response.jobId
                selected = emptySet()
            } catch (e: AppError) {
                message = e.message
            } catch (e: Exception) {
                message = e.message ?: "批量操作失败"
            } finally {
                busy = false
            }
        }
    }

    fun consumeJob() {
        pendingJobId = null
    }

    fun consumeMessage() {
        message = null
    }

    fun clearError() {
        error = null
    }
}
