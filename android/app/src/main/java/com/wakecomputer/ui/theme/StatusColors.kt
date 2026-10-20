package com.wakecomputer.ui.theme

import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.material3.MaterialTheme

object StatusPalette {
    val Online = Color(0xFF16A34A)
    val OnlineDark = Color(0xFF22C55E)
    val OnlineNoAgent = Color(0xFF0D9488)
    val OnlineNoAgentDark = Color(0xFF2DD4BF)
    val Waking = Color(0xFFD97706)
    val WakingDark = Color(0xFFF59E0B)
    val Offline = Color(0xFF64748B)
    val OfflineDark = Color(0xFF94A3B8)
    val Failed = Color(0xFFDC2626)
    val FailedDark = Color(0xFFF87171)
}

@Composable
fun statusColor(status: String): Color {
    val dark = LocalWakeDark.current
    return when (status) {
        "online" -> if (dark) StatusPalette.OnlineDark else StatusPalette.Online
        "online_no_agent" -> if (dark) StatusPalette.OnlineNoAgentDark else StatusPalette.OnlineNoAgent
        "waking" -> if (dark) StatusPalette.WakingDark else StatusPalette.Waking
        "offline" -> if (dark) StatusPalette.OfflineDark else StatusPalette.Offline
        else -> if (dark) StatusPalette.OfflineDark else StatusPalette.Offline
    }
}

fun statusLabel(status: String): String = when (status) {
    "online" -> "在线"
    "online_no_agent" -> "在线（无 Agent）"
    "waking" -> "唤醒中"
    "offline" -> "离线"
    else -> "未知"
}

@Composable
fun resultColor(result: String): Color {
    val dark = LocalWakeDark.current
    return when (result) {
        "ok", "succeeded" -> if (dark) StatusPalette.OnlineDark else StatusPalette.Online
        "failed", "canceled" -> if (dark) StatusPalette.FailedDark else StatusPalette.Failed
        "partial" -> if (dark) StatusPalette.WakingDark else StatusPalette.Waking
        else -> MaterialTheme.colorScheme.onSurfaceVariant
    }
}
