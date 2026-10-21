package com.wakecomputer.data

import android.content.Context

class AppContainer(context: Context) {
    val store = LocalStore(context)
    val apiProvider = ApiProvider()
    val repository = Repository(apiProvider, store)
}
