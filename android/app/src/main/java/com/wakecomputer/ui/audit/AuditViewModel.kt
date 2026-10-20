package com.wakecomputer.ui.audit

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.wakecomputer.data.AuditDto
import com.wakecomputer.data.Repository
import kotlinx.coroutines.launch

class AuditViewModel(private val repo: Repository) : ViewModel() {
    var items by mutableStateOf<List<AuditDto>>(emptyList())
        private set
    var loading by mutableStateOf(true)
        private set
    var loadingMore by mutableStateOf(false)
        private set
    var error by mutableStateOf<String?>(null)
        private set
    var nextCursor by mutableStateOf<Long?>(null)
        private set

    val hasMore: Boolean get() = nextCursor != null

    init {
        refresh()
    }

    fun refresh() {
        loading = true
        viewModelScope.launch {
            try {
                val res = repo.audit()
                items = res.items
                nextCursor = res.nextCursor
                error = null
            } catch (e: Exception) {
                error = e.message ?: "加载失败"
            } finally {
                loading = false
            }
        }
    }

    fun loadMore() {
        val cursor = nextCursor ?: return
        if (loadingMore) return
        loadingMore = true
        viewModelScope.launch {
            try {
                val res = repo.audit(cursor = cursor)
                items = items + res.items
                nextCursor = res.nextCursor
            } catch (e: Exception) {
                error = e.message ?: "加载失败"
            } finally {
                loadingMore = false
            }
        }
    }
}
