package com.wakecomputer.ui.login

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.wakecomputer.data.AppError
import com.wakecomputer.data.Repository
import kotlinx.coroutines.launch

class LoginViewModel(private val repo: Repository) : ViewModel() {
    var scheme by mutableStateOf("https")
    var host by mutableStateOf("")
    var username by mutableStateOf("admin")
    var password by mutableStateOf("")
    var rememberPassword by mutableStateOf(true)
        private set
    var loading by mutableStateOf(false)
        private set
    var error by mutableStateOf<String?>(null)
        private set

    init {
        viewModelScope.launch {
            repo.serverUrl()?.let { applySavedServer(it) }
            repo.savedCredentials()?.let { (savedUser, savedPassword) ->
                username = savedUser
                password = savedPassword
                rememberPassword = true
            }
        }
    }

    fun onRememberChange(value: Boolean) {
        rememberPassword = value
    }

    fun onHostChange(raw: String) {
        val lower = raw.lowercase()
        when {
            lower.startsWith("https://") -> {
                scheme = "https"
                host = raw.substring(8)
            }
            lower.startsWith("http://") -> {
                scheme = "http"
                host = raw.substring(7)
            }
            else -> host = raw
        }
    }

    fun login(onSuccess: () -> Unit) {
        val target = serverUrl()
        if (target == null) {
            error = "请填写域名或 IP"
            return
        }
        if (password.isEmpty()) {
            error = "请填写密码"
            return
        }
        if (loading) return
        loading = true
        error = null
        viewModelScope.launch {
            try {
                repo.login(target, username, password)
                if (rememberPassword) {
                    repo.rememberCredentials(username.trim(), password)
                } else {
                    repo.forgetCredentials()
                }
                onSuccess()
            } catch (e: AppError) {
                error = e.message
            } catch (e: Exception) {
                error = e.message ?: "登录失败"
            } finally {
                loading = false
            }
        }
    }

    private fun serverUrl(): String? {
        var value = host.trim().trimEnd('/')
        if (value.isEmpty()) return null
        val lower = value.lowercase()
        when {
            lower.startsWith("https://") -> {
                scheme = "https"
                value = value.substring(8)
            }
            lower.startsWith("http://") -> {
                scheme = "http"
                value = value.substring(7)
            }
        }
        value = value.trim().trimEnd('/')
        if (value.isEmpty() || value.contains("://") || value.any { it.isWhitespace() }) return null
        return "$scheme://$value"
    }

    private fun applySavedServer(url: String) {
        val trimmed = url.trim().trimEnd('/')
        when {
            trimmed.startsWith("https://", ignoreCase = true) -> {
                scheme = "https"
                host = trimmed.substring(8)
            }
            trimmed.startsWith("http://", ignoreCase = true) -> {
                scheme = "http"
                host = trimmed.substring(7)
            }
            else -> host = trimmed
        }
    }
}
