package com.wakecomputer.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import com.wakecomputer.R
import com.wakecomputer.data.AppContainer
import com.wakecomputer.ui.audit.AuditScreen
import com.wakecomputer.ui.batch.BatchScreen
import com.wakecomputer.ui.detail.HostDetailScreen
import com.wakecomputer.ui.edit.HostEditScreen
import com.wakecomputer.ui.hosts.HostListScreen
import com.wakecomputer.ui.login.LoginScreen
import com.wakecomputer.ui.settings.SettingsScreen

object Routes {
    const val LOGIN = "login"
    const val HOSTS = "hosts"
    const val HOST_DETAIL = "hosts/{id}"
    const val HOST_EDIT = "host-edit/{id}"
    const val JOB = "job/{id}"
    const val AUDIT = "audit"
    const val SETTINGS = "settings"

    fun detail(id: String) = "hosts/$id"
    fun edit(id: String?) = "host-edit/${id ?: "new"}"
    fun job(id: String) = "job/$id"
}

@Composable
fun AppRoot(container: AppContainer) {
    val nav = rememberNavController()
    var start by remember { mutableStateOf<String?>(null) }

    LaunchedEffect(Unit) {
        start = if (container.repository.restoreSession()) Routes.HOSTS else Routes.LOGIN
    }

    val startDestination = start
    if (startDestination == null) {
        SplashScreen()
        return
    }

    NavHost(navController = nav, startDestination = startDestination) {
        composable(Routes.LOGIN) {
            LoginScreen(container) {
                nav.navigate(Routes.HOSTS) {
                    popUpTo(Routes.LOGIN) { inclusive = true }
                }
            }
        }
        composable(Routes.HOSTS) {
            HostListScreen(
                container = container,
                onOpenHost = { nav.navigate(Routes.detail(it)) },
                onAddHost = { nav.navigate(Routes.edit(null)) },
                onOpenJob = { nav.navigate(Routes.job(it)) },
                onOpenAudit = { nav.navigate(Routes.AUDIT) },
                onOpenSettings = { nav.navigate(Routes.SETTINGS) }
            )
        }
        composable(Routes.HOST_DETAIL) { entry ->
            val id = entry.arguments?.getString("id") ?: return@composable
            HostDetailScreen(
                container = container,
                hostId = id,
                onBack = { nav.popBackStack() },
                onEdit = { nav.navigate(Routes.edit(it)) },
                onDeleted = { nav.popBackStack() }
            )
        }
        composable(Routes.HOST_EDIT) { entry ->
            val raw = entry.arguments?.getString("id")
            val id = if (raw == null || raw == "new") null else raw
            HostEditScreen(
                container = container,
                hostId = id,
                onBack = { nav.popBackStack() },
                onSaved = { nav.popBackStack() }
            )
        }
        composable(Routes.JOB) { entry ->
            val id = entry.arguments?.getString("id") ?: return@composable
            BatchScreen(container = container, jobId = id, onBack = { nav.popBackStack() })
        }
        composable(Routes.AUDIT) {
            AuditScreen(container = container, onBack = { nav.popBackStack() })
        }
        composable(Routes.SETTINGS) {
            SettingsScreen(
                container = container,
                onBack = { nav.popBackStack() },
                onLoggedOut = {
                    nav.navigate(Routes.LOGIN) {
                        popUpTo(nav.graph.id) { inclusive = true }
                    }
                }
            )
        }
    }
}

@Composable
private fun SplashScreen() {
    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(MaterialTheme.colorScheme.background),
        contentAlignment = Alignment.Center
    ) {
        Column(
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.Center
        ) {
            Box(
                modifier = Modifier
                    .size(80.dp)
                    .clip(RoundedCornerShape(22.dp))
                    .background(MaterialTheme.colorScheme.primary),
                contentAlignment = Alignment.Center
            ) {
                androidx.compose.foundation.Image(
                    painter = painterResource(R.drawable.ic_launcher_foreground),
                    contentDescription = null,
                    modifier = Modifier.size(80.dp)
                )
            }
            androidx.compose.foundation.layout.Spacer(Modifier.size(18.dp))
            Text(
                text = "Wake",
                fontSize = 24.sp,
                fontWeight = FontWeight.Bold,
                color = MaterialTheme.colorScheme.onBackground
            )
            androidx.compose.foundation.layout.Spacer(Modifier.size(22.dp))
            CircularProgressIndicator(modifier = Modifier.size(22.dp), strokeWidth = 2.dp)
        }
    }
}
