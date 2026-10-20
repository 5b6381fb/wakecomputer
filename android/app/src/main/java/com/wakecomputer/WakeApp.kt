package com.wakecomputer

import android.app.Application
import com.wakecomputer.data.AppContainer

class WakeApp : Application() {
    lateinit var container: AppContainer
        private set

    override fun onCreate() {
        super.onCreate()
        container = AppContainer(this)
    }
}
