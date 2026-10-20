package com.wakecomputer

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import com.wakecomputer.data.ThemeMode
import com.wakecomputer.ui.AppRoot
import com.wakecomputer.ui.theme.WakeTheme

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        val container = (application as WakeApp).container
        setContent {
            val theme by container.store.themeMode.collectAsState(initial = ThemeMode.SYSTEM)
            WakeTheme(theme) {
                AppRoot(container)
            }
        }
    }
}
