package com.wakecomputer.ui.detail

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowBack
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import com.wakecomputer.data.AppContainer
import com.wakecomputer.data.AuditDto
import com.wakecomputer.data.HostDto
import com.wakecomputer.ui.components.InfoRow
import com.wakecomputer.ui.components.SectionCard
import com.wakecomputer.ui.components.StatusDot
import com.wakecomputer.ui.components.osLabel
import com.wakecomputer.ui.components.relativeTime
import com.wakecomputer.ui.theme.resultColor
import com.wakecomputer.ui.theme.statusColor
import com.wakecomputer.ui.theme.statusLabel
import kotlinx.coroutines.launch

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HostDetailScreen(
    container: AppContainer,
    hostId: String,
    onBack: () -> Unit,
    onEdit: (String) -> Unit,
    onDeleted: () -> Unit
) {
    val vm: HostDetailViewModel = viewModel(
        key = hostId,
        factory = viewModelFactory { initializer { HostDetailViewModel(container.repository, hostId) } }
    )
    val snackbar = remember { SnackbarHostState() }
    val scope = rememberCoroutineScope()
    val clipboard = LocalClipboardManager.current
    var confirmPower by remember { mutableStateOf<String?>(null) }
    var confirmDelete by remember { mutableStateOf(false) }
    var confirmRemoveAgent by remember { mutableStateOf(false) }

    LaunchedEffect(Unit) { vm.startPolling() }
    DisposableEffect(Unit) { onDispose { vm.stopPolling() } }
    LaunchedEffect(vm.message) {
        vm.message?.let { snackbar.showSnackbar(it); vm.consumeMessage() }
    }

    val host = vm.host

    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Text(host?.name ?: "主机详情", fontWeight = FontWeight.SemiBold)
                },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.Default.ArrowBack, contentDescription = "返回")
                    }
                },
                actions = {
                    if (host != null) {
                        IconButton(onClick = { onEdit(host.id) }) {
                            Icon(Icons.Default.Edit, contentDescription = "编辑")
                        }
                        IconButton(onClick = { confirmDelete = true }) {
                            Icon(Icons.Default.Delete, contentDescription = "删除")
                        }
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.background
                )
            )
        },
        snackbarHost = { SnackbarHost(snackbar) }
    ) { padding ->
        when {
            vm.loading && host == null -> Row(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(padding),
                horizontalArrangement = Arrangement.Center,
                verticalAlignment = Alignment.CenterVertically
            ) { CircularProgressIndicator() }

            vm.error != null && host == null -> Column(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(padding)
                    .padding(32.dp),
                horizontalAlignment = Alignment.CenterHorizontally
            ) {
                Text(vm.error ?: "", color = MaterialTheme.colorScheme.error)
                Spacer(Modifier.height(12.dp))
                Button(onClick = vm::refresh) { Text("重试") }
            }

            host != null -> LazyColumn(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(padding),
                contentPadding = androidx.compose.foundation.layout.PaddingValues(bottom = 32.dp)
            ) {
                item { StatusHero(host) }
                item { ActionButtons(host, vm.busy, onWake = vm::wake, onPower = { confirmPower = it }) }
                item {
                    SectionCard("主机信息") {
                        InfoRow("系统", osLabel(host.osType))
                        InfoRow("IP 地址", host.ip.ifBlank { "未设置" })
                        InfoRow("MAC", host.mac)
                        InfoRow("广播地址", host.broadcastTargets.joinToString(", ").ifBlank { "默认" })
                        InfoRow("WoL 端口", host.wolPort.toString())
                        InfoRow("探测端口", if (host.probePort > 0) host.probePort.toString() else "未设置")
                        if (host.notes.isNotBlank()) InfoRow("备注", host.notes)
                    }
                }
                item {
                    SectionCard("Agent") {
                        if (host.agentVersion.isNullOrBlank()) {
                            Text(
                                "未安装 Agent：仅支持唤醒与状态探测，无法远程关机。",
                                style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant
                            )
                            Spacer(Modifier.height(10.dp))
                            Button(onClick = vm::requestEnrollCode, enabled = !vm.busy) {
                                Text("生成接入码")
                            }
                        } else {
                            InfoRow("版本", host.agentVersion ?: "")
                            InfoRow("最后心跳", relativeTime(host.lastSeenAt))
                            Spacer(Modifier.height(8.dp))
                            OutlinedButton(onClick = { confirmRemoveAgent = true }) {
                                Text("解绑 Agent")
                            }
                        }
                    }
                }
                item {
                    if (vm.audit.isNotEmpty()) {
                        SectionCard("最近操作") {
                            vm.audit.forEach { entry ->
                                AuditRow(entry)
                            }
                        }
                    }
                }
            }
        }
    }

    val powerAction = confirmPower
    if (powerAction != null) {
        AlertDialog(
            onDismissRequest = { confirmPower = null },
            title = { Text(if (powerAction == "shutdown") "确认关机" else "确认重启") },
            text = {
                Text(
                    "将对「${host?.name ?: ""}」执行${if (powerAction == "shutdown") "关机" else "重启"}。\n" +
                        "指令延迟 30 秒执行，期间可在主机详情页取消。"
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    confirmPower = null
                    vm.power(powerAction, 30)
                }) { Text("确认") }
            },
            dismissButton = { TextButton(onClick = { confirmPower = null }) { Text("取消") } }
        )
    }

    if (confirmDelete) {
        AlertDialog(
            onDismissRequest = { confirmDelete = false },
            title = { Text("删除主机") },
            text = { Text("确定删除「${host?.name ?: ""}」吗？该操作不可恢复。") },
            confirmButton = {
                TextButton(onClick = {
                    confirmDelete = false
                    vm.delete(onDeleted)
                }) { Text("删除", color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = { TextButton(onClick = { confirmDelete = false }) { Text("取消") } }
        )
    }

    if (confirmRemoveAgent) {
        AlertDialog(
            onDismissRequest = { confirmRemoveAgent = false },
            title = { Text("解绑 Agent") },
            text = { Text("解绑后该主机将无法远程关机/重启，需要重新安装 Agent。") },
            confirmButton = {
                TextButton(onClick = {
                    confirmRemoveAgent = false
                    vm.removeAgent()
                }) { Text("解绑") }
            },
            dismissButton = { TextButton(onClick = { confirmRemoveAgent = false }) { Text("取消") } }
        )
    }

    vm.enroll?.let { code ->
        AlertDialog(
            onDismissRequest = vm::dismissEnroll,
            title = { Text("安装 Agent") },
            text = {
                Column(
                    modifier = Modifier
                        .heightIn(max = 420.dp)
                        .verticalScroll(rememberScrollState())
                ) {
                    Text("接入码：${code.code}", fontWeight = FontWeight.SemiBold)
                    Text(
                        "10 分钟内有效，仅可使用一次。",
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                    Spacer(Modifier.height(12.dp))
                    Text("Linux：", style = MaterialTheme.typography.labelMedium)
                    SelectionContainer {
                        Text(
                            code.linuxCommand,
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    }
                    TextButton(onClick = {
                        clipboard.setText(AnnotatedString(code.linuxCommand))
                        scope.launch { snackbar.showSnackbar("已复制 Linux 命令") }
                    }) { Text("复制 Linux 命令") }

                    Spacer(Modifier.height(8.dp))
                    Text("Windows（管理员 PowerShell）：", style = MaterialTheme.typography.labelMedium)
                    SelectionContainer {
                        Text(
                            code.windowsCommand,
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    }
                    TextButton(onClick = {
                        clipboard.setText(AnnotatedString(code.windowsCommand))
                        scope.launch { snackbar.showSnackbar("已复制 Windows 命令") }
                    }) { Text("复制 Windows 命令") }
                }
            },
            confirmButton = { TextButton(onClick = vm::dismissEnroll) { Text("关闭") } }
        )
    }
}

@Composable
private fun StatusHero(host: HostDto) {
    Card(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 8.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        shape = RoundedCornerShape(18.dp)
    ) {
        Column(modifier = Modifier.padding(20.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                StatusDot(host.status, 14)
                Spacer(Modifier.width(10.dp))
                Text(
                    text = statusLabel(host.status),
                    fontSize = 24.sp,
                    fontWeight = FontWeight.Bold,
                    color = statusColor(host.status)
                )
            }
            Spacer(Modifier.height(6.dp))
            Text(
                text = "最后在线：${relativeTime(host.lastSeenAt)}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
    }
}

@Composable
private fun ActionButtons(
    host: HostDto,
    busy: Boolean,
    onWake: () -> Unit,
    onPower: (String) -> Unit
) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 6.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Button(
                onClick = onWake,
                enabled = !busy,
                modifier = Modifier
                    .weight(1f)
                    .height(46.dp),
                shape = RoundedCornerShape(13.dp)
            ) { Text("唤醒") }
            Button(
                onClick = { onPower("shutdown") },
                enabled = !busy && host.capabilities.shutdown,
                modifier = Modifier
                    .weight(1f)
                    .height(46.dp),
                shape = RoundedCornerShape(13.dp),
                colors = ButtonDefaults.buttonColors(
                    containerColor = MaterialTheme.colorScheme.error,
                    contentColor = MaterialTheme.colorScheme.onError
                )
            ) { Text("关机") }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            FilledTonalButton(
                onClick = { onPower("reboot") },
                enabled = !busy && host.capabilities.reboot,
                modifier = Modifier
                    .weight(1f)
                    .height(46.dp),
                shape = RoundedCornerShape(13.dp)
            ) { Text("重启") }
            OutlinedButton(
                onClick = { onPower("cancel") },
                enabled = !busy && host.capabilities.cancel,
                modifier = Modifier
                    .weight(1f)
                    .height(46.dp),
                shape = RoundedCornerShape(13.dp)
            ) { Text("取消关机") }
        }
        if (!host.capabilities.shutdown) {
            Text(
                "安装 Agent 后可远程关机/重启",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
    }
}

private fun actionLabel(action: String): String = when (action) {
    "host.wake" -> "唤醒"
    "host.shutdown" -> "关机"
    "host.reboot" -> "重启"
    "host.cancel" -> "取消关机"
    "host.create" -> "新增主机"
    "host.update" -> "编辑主机"
    "host.delete" -> "删除主机"
    "agent.enroll" -> "Agent 接入"
    "agent.enroll_code" -> "生成接入码"
    "agent.remove" -> "解绑 Agent"
    "auth.login" -> "登录"
    else -> action
}

@Composable
private fun AuditRow(entry: AuditDto) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(vertical = 6.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Text(
            text = actionLabel(entry.action),
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier.weight(1f)
        )
        Text(
            text = entry.result,
            style = MaterialTheme.typography.labelSmall,
            color = resultColor(entry.result)
        )
        Spacer(Modifier.width(10.dp))
        Text(
            text = relativeTime(entry.createdAt),
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
    }
}
