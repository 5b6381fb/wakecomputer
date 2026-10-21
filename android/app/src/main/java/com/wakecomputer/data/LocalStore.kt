package com.wakecomputer.data

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map

private val Context.dataStore: DataStore<Preferences> by preferencesDataStore(name = "wakecomputer")

enum class ThemeMode {
    SYSTEM, LIGHT, DARK;

    companion object {
        fun from(raw: String?): ThemeMode = entries.firstOrNull { it.name == raw } ?: SYSTEM
    }
}

class LocalStore(private val context: Context) {
    private object Keys {
        val SERVER = stringPreferencesKey("server_url")
        val REFRESH = stringPreferencesKey("refresh_token")
        val THEME = stringPreferencesKey("theme_mode")
        val HOSTS = stringPreferencesKey("hosts_cache")
        val REMEMBER = booleanPreferencesKey("remember_password")
        val SAVED_USER = stringPreferencesKey("saved_username")
        val SAVED_PASSWORD = stringPreferencesKey("saved_password")
    }

    val themeMode: Flow<ThemeMode> = context.dataStore.data.map { ThemeMode.from(it[Keys.THEME]) }
    val serverUrl: Flow<String?> = context.dataStore.data.map { it[Keys.SERVER] }

    suspend fun saveServer(server: String) {
        context.dataStore.edit { it[Keys.SERVER] = server }
    }

    suspend fun saveTheme(mode: ThemeMode) {
        context.dataStore.edit { it[Keys.THEME] = mode.name }
    }

    suspend fun saveSession(server: String, refreshToken: String) {
        val encrypted = SecureStore.encrypt(refreshToken) ?: return
        context.dataStore.edit {
            it[Keys.SERVER] = server
            it[Keys.REFRESH] = encrypted
        }
    }

    suspend fun server(): String? = context.dataStore.data.map { it[Keys.SERVER] }.first()

    suspend fun refreshToken(): String? {
        val encrypted = context.dataStore.data.map { it[Keys.REFRESH] }.first() ?: return null
        return SecureStore.decrypt(encrypted)
    }

    suspend fun clearSession() {
        context.dataStore.edit { it.remove(Keys.REFRESH) }
    }

    /** Saved login form, encrypted with the Android Keystore. */
    suspend fun savedCredentials(): Pair<String, String>? {
        val data = context.dataStore.data.first()
        if (data[Keys.REMEMBER] != true) return null
        val user = data[Keys.SAVED_USER] ?: return null
        val password = data[Keys.SAVED_PASSWORD]?.let { SecureStore.decrypt(it) } ?: return null
        return user to password
    }

    suspend fun rememberCredentials(username: String, password: String) {
        val encrypted = SecureStore.encrypt(password) ?: return
        context.dataStore.edit {
            it[Keys.REMEMBER] = true
            it[Keys.SAVED_USER] = username
            it[Keys.SAVED_PASSWORD] = encrypted
        }
    }

    suspend fun forgetCredentials() {
        context.dataStore.edit {
            it[Keys.REMEMBER] = false
            it.remove(Keys.SAVED_USER)
            it.remove(Keys.SAVED_PASSWORD)
        }
    }

    suspend fun saveHosts(json: String) {
        context.dataStore.edit { it[Keys.HOSTS] = json }
    }

    suspend fun hostsJson(): String? =
        context.dataStore.data.map { it[Keys.HOSTS] }.first()
}
