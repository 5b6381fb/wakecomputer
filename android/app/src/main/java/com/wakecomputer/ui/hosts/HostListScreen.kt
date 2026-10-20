package com.wakecomputer.ui.hosts

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.List
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.BottomAppBar
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
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
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import com.wakecomputer.data.AppContainer
import com.wakecomputer.ui.components.HostCard
import com.wakecomputer.ui.theme.statusColor

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HostListScreen(
    container: AppContainer,
    onOpenHost: (String) -> Unit,
    onAddHost: () -> Unit,
    onOpenJob: (String) -> Unit,
    onOpenAudit: () -> Unit,
    onOpenSettings: () -> Unit
) {
    val vm: HostListViewModel = viewModel(
        factory = viewModelFactory { initializer { HostListViewModel(container.repository) } }
    )
    val snackbar = remember { SnackbarHostState() }
    var confirmAction by remember { mutableStateOf<String?>(null) }

    LaunchedEffect(Unit) { vm.startPolling() }
    DisposableEffect(Unit) { onDispose { vm.stopPolling() } }
    LaunchedEffect(vm.pendingJobId) {
        vm.pendingJobId?.let { vm.consumeJob(); onOpenJob(it) }
    }
    LaunchedEffect(vm.message) {
        vm.message?.let { snackbar.showSnackbar(it); vm.consumeMessage() }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Text(
                        if (vm.selectionMode) "已选 ${vm.selected.size} 台" else "主机",
                        fontWeight = FontWeight.SemiBold
                    )
                },
                navigationIcon = {
                    if (vm.selectionMode) {
                        IconButton(onClick = vm::clearSelection) {
                            Icon(Icons.Default.Close, contentDescription = "退出多选")
                        }
                    }
                },
                actions = {
                    if (vm.selectionMode) {
                        TextButton(onClick = vm::selectAll) { Text("全选") }
                    } else {
                        IconButton(onClick = vm::refresh) {
                            Icon(Icons.Default.Refresh, contentDescription = "刷新")
                        }
                        IconButton(onClick = onOpenAudit) {
                            Icon(Icons.Default.List, contentDescription = "审计日志")
                        }
                        IconButton(onClick = onOpenSettings) {
                            Icon(Icons.Default.Settings, contentDescription = "设置")
                        }
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.background
                )
            )
        },
        snackbarHost = { SnackbarHost(snackbar) },
        floatingActionButton = {
            if (!vm.selectionMode) {
                FloatingActionButton(onClick = onAddHost) {
                    Icon(Icons.Default.Add, contentDescription = "添加主机")
                }
            }
        },
        bottomBar = {
            if (vm.selectionMode) {
                BottomAppBar(containerColor = MaterialTheme.colorScheme.surface) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceEvenly
                    ) {
                        TextButton(onClick = { vm.runBatch("wake") }, enabled = !vm.busy) {
                            Text("唤醒")
                        }
                        TextButton(onClick = { confirmAction = "shutdown" }, enabled = !vm.busy) {
                            Text("关机")
                        }
                        TextButton(onClick = { confirmAction = "reboot" }, enabled = !vm.busy) {
                            Text("重启")
                        }
                        TextButton(onClick = { vm.runBatch("cancel") }, enabled = !vm.busy) {
                            Text("取消关机")
                        }
                    }
                }
            }
        }
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
        ) {
            SummaryRow(
                online = vm.hosts.count { it.status == "online" || it.status == "online_no_agent" },
                waking = vm.hosts.count { it.status == "waking" },
                total = vm.hosts.size
            )

            if (vm.fromCache) {
                Text(
                    text = "离线缓存 · 数据可能不是最新",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.padding(horizontal = 20.dp, vertical = 2.dp)
                )
            }

            OutlinedTextField(
                value = vm.query,
                onValueChange = { vm.query = it },
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp),
                placeholder = { Text("搜索名称、IP 或备注") },
                leadingIcon = { Icon(Icons.Default.Search, contentDescription = null) },
                singleLine = true,
                shape = RoundedCornerShape(14.dp)
            )

            if (vm.groups.isNotEmpty()) {
                LazyRow(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 12.dp, vertical = 8.dp),
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    item {
                        FilterChip(
                            selected = vm.groupFilter == null,
                            onClick = { vm.groupFilter = null },
                            label = { Text("全部") }
                        )
                    }
                    items(vm.groups, key = { it.id }) { group ->
                        FilterChip(
                            selected = vm.groupFilter == group.id,
                            onClick = {
                                vm.groupFilter = if (vm.groupFilter == group.id) null else group.id
                            },
                            label = { Text(group.name) }
                        )
                    }
                }
            } else {
                Spacer(Modifier.height(8.dp))
            }

            when {
                vm.loading && vm.hosts.isEmpty() -> {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(40.dp),
                        horizontalArrangement = Arrangement.Center
                    ) { CircularProgressIndicator() }
                }

                vm.error != null && vm.hosts.isEmpty() -> {
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(32.dp),
                        horizontalAlignment = Alignment.CenterHorizontally
                    ) {
                        Text(
                            text = vm.error ?: "",
                            color = MaterialTheme.colorScheme.error,
                            style = MaterialTheme.typography.bodyMedium
                        )
                        Spacer(Modifier.height(12.dp))
                        Button(onClick = {
                            vm.clearError()
                            vm.refresh()
                        }) { Text("重试") }
                    }
                }

                vm.filtered.isEmpty() -> {
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(48.dp),
                        horizontalAlignment = Alignment.CenterHorizontally
                    ) {
                        Text(
                            text = if (vm.hosts.isEmpty()) "还没有主机" else "没有匹配的主机",
                            style = MaterialTheme.typography.titleMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant
                        )
                        Spacer(Modifier.height(8.dp))
                        Text(
                            text = if (vm.hosts.isEmpty()) "点击右下角 + 添加第一台主机" else "试试换个关键词",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    }
                }

                else -> {
                    LazyColumn(
                        modifier = Modifier.fillMaxSize(),
                        contentPadding = androidx.compose.foundation.layout.PaddingValues(
                            top = 4.dp, bottom = 88.dp
                        )
                    ) {
                        items(vm.filtered, key = { it.id }) { host ->
                            HostCard(
                                host = host,
                                selectionMode = vm.selectionMode,
                                selected = host.id in vm.selected,
                                onClick = {
                                    if (vm.selectionMode) vm.toggle(host.id) else onOpenHost(host.id)
                                },
                                onLongClick = { vm.toggle(host.id) }
                            )
                        }
                    }
                }
            }
        }
    }

    val action = confirmAction
    if (action != null) {
        val isShutdown = action == "shutdown"
        AlertDialog(
            onDismissRequest = { confirmAction = null },
            title = { Text(if (isShutdown) "批量关机" else "批量重启") },
            text = {
                Text(
                    "将对 ${vm.selected.size} 台主机执行${if (isShutdown) "关机" else "重启"}。\n" +
                        "指令延迟 30 秒执行，可在任务页面取消。\n" +
                        "未安装 Agent 的主机会被标记为失败。"
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    confirmAction = null
                    vm.runBatch(action, 30)
                }) {
                    Text(if (isShutdown) "确认关机" else "确认重启")
                }
            },
            dismissButton = {
                TextButton(onClick = { confirmAction = null }) { Text("取消") }
            }
        )
    }
}

@Composable
private fun SummaryRow(online: Int, waking: Int, total: Int) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 20.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Text(
            text = "在线 $online",
            style = MaterialTheme.typography.labelLarge,
            color = statusColor("online")
        )
        Spacer(Modifier.width(14.dp))
        if (waking > 0) {
            Text(
                text = "唤醒中 $waking",
                style = MaterialTheme.typography.labelLarge,
                color = statusColor("waking")
            )
            Spacer(Modifier.width(14.dp))
        }
        Text(
            text = "共 $total 台",
            style = MaterialTheme.typography.labelLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
    }
}
