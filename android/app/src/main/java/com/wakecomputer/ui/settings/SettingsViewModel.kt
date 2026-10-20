package com.wakecomputer.ui.settings

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.wakecomputer.data.GroupDto
import com.wakecomputer.data.LocalStore
import com.wakecomputer.data.Repository
import com.wakecomputer.data.ThemeMode
import kotlinx.coroutines.launch

class SettingsViewModel(
    private val repo: Repository,
    private val store: LocalStore
) : ViewModel() {
    var theme by mutableStateOf(ThemeMode.SYSTEM)
        private set
    var groups by mutableStateOf<List<GroupDto>>(emptyList())
        private set
    var server by mutableStateOf("")
        private set
    var serverVersion by mutableStateOf("")
        private set
    var username by mutableStateOf("")
        private set
    var loading by mutableStateOf(true)
        private set
    var message by mutableStateOf<String?>(null)
    var error by mutableStateOf<String?>(null)
        private set
    var working by mutableStateOf(false)
        private set

    init {
        viewModelScope.launch {
            store.themeMode.collect { theme = it }
        }
        viewModelScope.launch {
            server = repo.serverUrl() ?: ""
            try {
                val me = repo.me()
                username = me.user.username
                serverVersion = me.version
            } catch (e: Exception) {
                // handled by the global login flow when the token expires
            }
            try {
                groups = repo.groups()
            } catch (e: Exception) {
                error = e.message
            }
            loading = false
        }
    }

    fun updateTheme(mode: ThemeMode) {
        viewModelScope.launch { store.saveTheme(mode) }
    }

    fun addGroup(name: String, onDone: () -> Unit) {
        val trimmed = name.trim()
        if (trimmed.isEmpty()) {
            message = "分组名称不能为空"
            return
        }
        if (working) return
        working = true
        viewModelScope.launch {
            try {
                repo.createGroup(trimmed)
                groups = repo.groups()
                onDone()
            } catch (e: Exception) {
                message = e.message ?: "创建分组失败"
            } finally {
                working = false
            }
        }
    }

    fun deleteGroup(id: String) {
        viewModelScope.launch {
            try {
                repo.deleteGroup(id)
                groups = repo.groups()
            } catch (e: Exception) {
                message = e.message ?: "删除分组失败"
            }
        }
    }

    fun changePassword(old: String, new: String, confirm: String, onDone: () -> Unit) {
        if (new.length < 8) {
            message = "新密码至少 8 位"
            return
        }
        if (new != confirm) {
            message = "两次输入的密码不一致"
            return
        }
        if (working) return
        working = true
        viewModelScope.launch {
            try {
                repo.changePassword(old, new)
                message = "密码已更新，请重新登录"
                onDone()
            } catch (e: Exception) {
                message = e.message ?: "修改密码失败"
            } finally {
                working = false
            }
        }
    }

    fun logout(onDone: () -> Unit) {
        viewModelScope.launch {
            repo.logout()
            onDone()
        }
    }

    fun consumeMessage() {
        message = null
    }
}
