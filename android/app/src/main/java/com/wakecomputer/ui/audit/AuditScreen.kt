package com.wakecomputer.ui.audit

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowBack
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import com.wakecomputer.data.AppContainer
import com.wakecomputer.data.AuditDto
import com.wakecomputer.ui.components.relativeTime
import com.wakecomputer.ui.theme.resultColor

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AuditScreen(
    container: AppContainer,
    onBack: () -> Unit
) {
    val vm: AuditViewModel = viewModel(
        factory = viewModelFactory { initializer { AuditViewModel(container.repository) } }
    )

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("审计日志", fontWeight = FontWeight.SemiBold) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.Default.ArrowBack, contentDescription = "返回")
                    }
                },
                actions = {
                    IconButton(onClick = vm::refresh) {
                        Icon(Icons.Default.Refresh, contentDescription = "刷新")
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.background
                )
            )
        }
    ) { padding ->
        when {
            vm.loading && vm.items.isEmpty() -> Row(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(padding),
                horizontalArrangement = Arrangement.Center,
                verticalAlignment = Alignment.CenterVertically
            ) { CircularProgressIndicator() }

            vm.error != null && vm.items.isEmpty() -> Column(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(padding)
                    .padding(32.dp),
                horizontalAlignment = Alignment.CenterHorizontally
            ) {
                Text(vm.error ?: "", color = MaterialTheme.colorScheme.error)
                Spacer(Modifier.height(12.dp))
                TextButton(onClick = vm::refresh) { Text("重试") }
            }

            else -> LazyColumn(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(padding)
            ) {
                items(vm.items, key = { it.id }) { entry ->
                    AuditRow(entry)
                    HorizontalDivider(
                        modifier = Modifier.padding(horizontal = 20.dp),
                        color = MaterialTheme.colorScheme.surfaceVariant
                    )
                }
                if (vm.hasMore) {
                    item {
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.Center
                        ) {
                            TextButton(onClick = vm::loadMore, enabled = !vm.loadingMore) {
                                Text(if (vm.loadingMore) "加载中..." else "加载更多")
                            }
                        }
                    }
                }
            }
        }
    }
}

private fun actionLabel(action: String): String = when {
    action == "auth.login" -> "登录"
    action == "auth.login_failed" -> "登录失败"
    action == "auth.refresh_reuse" -> "令牌重放"
    action == "auth.password" -> "修改密码"
    action == "host.create" -> "新增主机"
    action == "host.update" -> "编辑主机"
    action == "host.delete" -> "删除主机"
    action == "host.wake" -> "唤醒"
    action == "host.shutdown" -> "关机"
    action == "host.reboot" -> "重启"
    action == "host.cancel" -> "取消关机"
    action == "agent.enroll" -> "Agent 接入"
    action == "agent.enroll_code" -> "生成接入码"
    action == "agent.remove" -> "解绑 Agent"
    action == "group.create" -> "新增分组"
    action == "group.update" -> "编辑分组"
    action == "group.delete" -> "删除分组"
    action == "settings.update" -> "修改设置"
    action.startsWith("batch.create") -> "创建批量任务"
    action.startsWith("batch.cancel") -> "取消批量任务"
    action.startsWith("batch.") -> "批量操作"
    else -> action
}

@Composable
private fun AuditRow(entry: AuditDto) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 20.dp, vertical = 11.dp)
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(
                text = actionLabel(entry.action),
                style = MaterialTheme.typography.bodyMedium,
                fontWeight = FontWeight.Medium,
                modifier = Modifier.weight(1f)
            )
            Text(
                text = entry.result,
                style = MaterialTheme.typography.labelSmall,
                color = resultColor(entry.result)
            )
        }
        Spacer(Modifier.height(3.dp))
        Text(
            text = buildString {
                if (entry.hostName.isNotBlank()) append(entry.hostName)
                if (entry.username.isNotBlank()) {
                    if (isNotEmpty()) append(" · ")
                    append(entry.username)
                }
                if (entry.ip.isNotBlank()) {
                    if (isNotEmpty()) append(" · ")
                    append(entry.ip)
                }
            },
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
        Spacer(Modifier.height(2.dp))
        Text(
            text = relativeTime(entry.createdAt),
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
    }
}
