package com.wakecomputer.ui.batch

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
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowBack
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import com.wakecomputer.data.AppContainer
import com.wakecomputer.data.JobDto
import com.wakecomputer.data.JobResultDto
import com.wakecomputer.ui.theme.resultColor

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun BatchScreen(
    container: AppContainer,
    jobId: String,
    onBack: () -> Unit
) {
    val vm: BatchViewModel = viewModel(
        key = jobId,
        factory = viewModelFactory { initializer { BatchViewModel(container.repository, jobId) } }
    )

    LaunchedEffect(Unit) { vm.start() }
    DisposableEffect(Unit) { onDispose { vm.stop() } }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("批量任务", fontWeight = FontWeight.SemiBold) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.Default.ArrowBack, contentDescription = "返回")
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.background
                )
            )
        }
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
        ) {
            val job = vm.job
            if (job == null) {
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(40.dp),
                    horizontalArrangement = Arrangement.Center
                ) {
                    if (vm.error == null) CircularProgressIndicator()
                    else Text(vm.error ?: "", color = MaterialTheme.colorScheme.error)
                }
                return@Column
            }

            JobSummary(job)

            if (job.status == "running" && (job.action == "shutdown" || job.action == "reboot")) {
                OutlinedButton(
                    onClick = vm::cancel,
                    enabled = !vm.canceling,
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp, vertical = 6.dp),
                    shape = RoundedCornerShape(13.dp)
                ) { Text("取消本次批量操作") }
            }

            vm.error?.let {
                Text(
                    it,
                    color = MaterialTheme.colorScheme.error,
                    style = MaterialTheme.typography.bodySmall,
                    modifier = Modifier.padding(horizontal = 20.dp, vertical = 6.dp)
                )
            }

            LazyColumn(
                modifier = Modifier.fillMaxSize(),
                contentPadding = androidx.compose.foundation.layout.PaddingValues(bottom = 24.dp)
            ) {
                items(job.results, key = { it.hostId }) { result ->
                    ResultRow(result)
                }
            }
        }
    }
}

private fun actionLabel(action: String): String = when (action) {
    "wake" -> "批量开机"
    "shutdown" -> "批量关机"
    "reboot" -> "批量重启"
    "cancel" -> "批量取消关机"
    else -> action
}

private fun statusLabel(status: String): String = when (status) {
    "running" -> "执行中"
    "succeeded" -> "全部成功"
    "partial" -> "部分成功"
    "failed" -> "执行失败"
    "canceled" -> "已取消"
    else -> status
}

private fun resultLabel(status: String): String = when (status) {
    "queued" -> "等待"
    "running" -> "执行中"
    "ok" -> "成功"
    "failed" -> "失败"
    "canceled" -> "已取消"
    else -> status
}

@Composable
private fun JobSummary(job: JobDto) {
    Card(
        modifier = Modifier
            .fillMaxWidth()
            .padding(16.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        shape = RoundedCornerShape(18.dp)
    ) {
        Column(modifier = Modifier.padding(18.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    text = actionLabel(job.action),
                    style = MaterialTheme.typography.titleMedium,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier.weight(1f)
                )
                Text(
                    text = statusLabel(job.status),
                    style = MaterialTheme.typography.labelLarge,
                    color = resultColor(job.status)
                )
            }
            Spacer(Modifier.height(12.dp))
            val fraction = if (job.total == 0) 0f else job.done.toFloat() / job.total
            LinearProgressIndicator(
                progress = { fraction },
                modifier = Modifier
                    .fillMaxWidth()
                    .height(6.dp),
                strokeCap = androidx.compose.ui.graphics.StrokeCap.Round
            )
            Spacer(Modifier.height(10.dp))
            Text(
                text = "完成 ${job.done}/${job.total} · 成功 ${job.succeeded} · 失败 ${job.failed}" +
                    if (job.canceled > 0) " · 取消 ${job.canceled}" else "",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            if (job.status == "running" && job.delaySeconds > 0) {
                Spacer(Modifier.height(4.dp))
                Text(
                    text = "指令将在 ${job.delaySeconds} 秒后于各主机本地执行",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }
        }
    }
}

@Composable
private fun ResultRow(result: JobResultDto) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 20.dp, vertical = 7.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(result.name, style = MaterialTheme.typography.bodyMedium)
            if (!result.error.isNullOrBlank()) {
                Text(
                    result.error ?: "",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.error
                )
            }
        }
        Spacer(Modifier.width(8.dp))
        Text(
            text = resultLabel(result.status),
            style = MaterialTheme.typography.labelMedium,
            color = resultColor(result.status)
        )
    }
}
