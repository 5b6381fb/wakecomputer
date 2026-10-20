package com.wakecomputer.ui.edit

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowBack
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import com.wakecomputer.data.AppContainer

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HostEditScreen(
    container: AppContainer,
    hostId: String?,
    onBack: () -> Unit,
    onSaved: (String) -> Unit
) {
    val vm: HostEditViewModel = viewModel(
        key = hostId ?: "new",
        factory = viewModelFactory {
            initializer { HostEditViewModel(container.repository, hostId) }
        }
    )

    LaunchedEffect(Unit) { /* initial load happens in the view model */ }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(if (vm.isNew) "添加主机" else "编辑主机", fontWeight = FontWeight.SemiBold) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.Default.ArrowBack, contentDescription = "返回")
                    }
                },
                actions = {
                    TextButton(onClick = { vm.save(onSaved) }, enabled = !vm.saving) {
                        Text("保存")
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.background
                )
            )
        }
    ) { padding ->
        if (vm.loading) {
            Row(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(padding),
                horizontalArrangement = Arrangement.Center,
                verticalAlignment = Alignment.CenterVertically
            ) { CircularProgressIndicator() }
            return@Scaffold
        }

        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .imePadding()
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp, vertical = 8.dp)
        ) {
            FilledTonalButton(
                onClick = vm::discover,
                enabled = !vm.discovering,
                modifier = Modifier.fillMaxWidth(),
                shape = RoundedCornerShape(14.dp)
            ) {
                if (vm.discovering) {
                    CircularProgressIndicator(
                        modifier = Modifier.size(18.dp),
                        strokeWidth = 2.dp,
                        color = MaterialTheme.colorScheme.onSecondaryContainer
                    )
                    Spacer(Modifier.width(10.dp))
                    Text("正在扫描局域网…")
                } else {
                    Text("扫描局域网设备")
                }
            }
            Spacer(Modifier.height(6.dp))
            Text(
                "自动发现同一网段内的设备，选中后自动填写 MAC / IP；也可以在下方手动填写。",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Spacer(Modifier.height(14.dp))

            OutlinedTextField(
                value = vm.name,
                onValueChange = { vm.name = it },
                label = { Text("主机名称 *") },
                placeholder = { Text("书房主机") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
                shape = RoundedCornerShape(14.dp)
            )
            Spacer(Modifier.height(12.dp))

            Text("操作系统", style = MaterialTheme.typography.labelMedium)
            Spacer(Modifier.height(6.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                listOf("windows" to "Windows", "linux" to "Linux", "other" to "其他").forEach { (key, label) ->
                    FilterChip(
                        selected = vm.osType == key,
                        onClick = { vm.osType = key },
                        label = { Text(label) }
                    )
                }
            }
            Spacer(Modifier.height(12.dp))

            OutlinedTextField(
                value = vm.mac,
                onValueChange = { vm.mac = it },
                label = { Text("MAC 地址 *") },
                placeholder = { Text("AA:BB:CC:DD:EE:FF") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
                shape = RoundedCornerShape(14.dp),
                supportingText = { Text("网卡 MAC，用于发送魔术包") }
            )
            Spacer(Modifier.height(12.dp))

            OutlinedTextField(
                value = vm.ip,
                onValueChange = { vm.ip = it },
                label = { Text("IP 地址") },
                placeholder = { Text("192.168.1.20") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
                shape = RoundedCornerShape(14.dp),
                supportingText = { Text("建议在路由器上做 DHCP 静态绑定") }
            )
            Spacer(Modifier.height(12.dp))

            OutlinedTextField(
                value = vm.broadcast,
                onValueChange = { vm.broadcast = it },
                label = { Text("广播地址") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
                shape = RoundedCornerShape(14.dp),
                supportingText = { Text("多个地址用逗号分隔；保持默认即可") }
            )
            Spacer(Modifier.height(12.dp))

            Row {
                OutlinedTextField(
                    value = vm.wolPort,
                    onValueChange = { vm.wolPort = it },
                    label = { Text("WoL 端口") },
                    singleLine = true,
                    modifier = Modifier.weight(1f),
                    shape = RoundedCornerShape(14.dp),
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number)
                )
                Spacer(Modifier.width(12.dp))
                OutlinedTextField(
                    value = vm.probePort,
                    onValueChange = { vm.probePort = it },
                    label = { Text("探测端口") },
                    placeholder = { Text("可空") },
                    singleLine = true,
                    modifier = Modifier.weight(1f),
                    shape = RoundedCornerShape(14.dp),
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number)
                )
            }
            Spacer(Modifier.height(12.dp))

            OutlinedTextField(
                value = vm.notes,
                onValueChange = { vm.notes = it },
                label = { Text("备注") },
                modifier = Modifier.fillMaxWidth(),
                shape = RoundedCornerShape(14.dp),
                minLines = 2,
                maxLines = 4
            )

            if (vm.groups.isNotEmpty()) {
                Spacer(Modifier.height(16.dp))
                Text("分组", style = MaterialTheme.typography.labelMedium)
                Spacer(Modifier.height(6.dp))
                LazyRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    items(vm.groups, key = { it.id }) { group ->
                        FilterChip(
                            selected = group.id in vm.selectedGroups,
                            onClick = { vm.toggleGroup(group.id) },
                            label = { Text(group.name) }
                        )
                    }
                }
            }

            if (vm.error != null) {
                Spacer(Modifier.height(14.dp))
                Text(
                    text = vm.error ?: "",
                    color = MaterialTheme.colorScheme.error,
                    style = MaterialTheme.typography.bodySmall
                )
            }

            Spacer(Modifier.height(24.dp))
        }
    }

    if (vm.showDiscovery) {
        ModalBottomSheet(onDismissRequest = vm::dismissDiscovery) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp)
                    .padding(bottom = 24.dp)
            ) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Column(modifier = Modifier.weight(1f)) {
                        Text(
                            "局域网设备",
                            style = MaterialTheme.typography.titleMedium,
                            fontWeight = FontWeight.SemiBold
                        )
                        Text(
                            "由服务端扫描，选择后自动填表",
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    }
                    TextButton(onClick = vm::discover, enabled = !vm.discovering) {
                        Text("重新扫描")
                    }
                }

                if (vm.discovering) {
                    LinearProgressIndicator(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(vertical = 10.dp)
                    )
                }

                vm.discoveryError?.let { message ->
                    Text(
                        text = message,
                        color = MaterialTheme.colorScheme.error,
                        style = MaterialTheme.typography.bodySmall,
                        modifier = Modifier.padding(vertical = 10.dp)
                    )
                }

                if (!vm.discovering && vm.discoveryError == null && vm.discovered.isEmpty()) {
                    Text(
                        "未发现设备。请确认目标主机在线，且与服务器处于同一网段。",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(vertical = 16.dp)
                    )
                }

                LazyColumn(modifier = Modifier.heightIn(max = 420.dp)) {
                    items(vm.discovered, key = { it.mac }) { device ->
                        ListItem(
                            headlineContent = {
                                Text(device.hostname.ifBlank { "未知设备" })
                            },
                            supportingContent = {
                                Text("${device.ip}  ·  ${device.mac}")
                            },
                            trailingContent = {
                                Text(
                                    text = if (device.added) "已添加" else "选择",
                                    style = MaterialTheme.typography.labelMedium,
                                    color = if (device.added) {
                                        MaterialTheme.colorScheme.onSurfaceVariant
                                    } else {
                                        MaterialTheme.colorScheme.primary
                                    }
                                )
                            },
                            modifier = Modifier.clickable(enabled = !device.added) {
                                vm.applyDevice(device)
                            }
                        )
                        HorizontalDivider(color = MaterialTheme.colorScheme.surfaceVariant)
                    }
                }
            }
        }
    }
}
