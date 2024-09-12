package tech.magichacker.myrtcdemo

import android.os.Bundle
import androidx.appcompat.app.AppCompatActivity
import tech.magichacker.myrtcdemo.databinding.ActivityConnectionBinding

class WebSocketModeActivity : AppCompatActivity() {
    private lateinit var binding: ActivityConnectionBinding

    private var client: PeerConnectionClient? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        binding = ActivityConnectionBinding.inflate(layoutInflater)
        setContentView(binding.root)

        client = PeerConnectionClient(applicationContext)
        client?.start()
    }

    override fun onDestroy() {
        super.onDestroy()
        client?.stop()
    }
}