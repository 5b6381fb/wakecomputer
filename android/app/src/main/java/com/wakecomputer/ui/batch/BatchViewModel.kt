package com.wakecomputer.ui.batch

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.wakecomputer.data.JobDto
import com.wakecomputer.data.Repository
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

class BatchViewModel(
    private val repo: Repository,
    private val jobId: String
) : ViewModel() {
    var job by mutableStateOf<JobDto?>(null)
        private set
    var error by mutableStateOf<String?>(null)
        private set
    var canceling by mutableStateOf(false)
        private set

    private var pollJob: Job? = null

    fun start() {
        if (pollJob != null) return
        pollJob = viewModelScope.launch {
            while (isActive) {
                try {
                    job = repo.job(jobId)
                    error = null
                } catch (e: Exception) {
                    error = e.message ?: "读取任务失败"
                }
                if (job?.status != "running") break
                delay(700)
            }
        }
    }

    fun stop() {
        pollJob?.cancel()
        pollJob = null
    }

    fun cancel() {
        if (canceling) return
        canceling = true
        viewModelScope.launch {
            try {
                job = repo.cancelJob(jobId)
            } catch (e: Exception) {
                error = e.message ?: "取消失败"
            } finally {
                canceling = false
                pollJob?.cancel()
                pollJob = null
                start()
            }
        }
    }
}
