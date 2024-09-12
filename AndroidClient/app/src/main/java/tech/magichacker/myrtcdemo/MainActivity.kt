package tech.magichacker.myrtcdemo

import android.app.Activity
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Bundle
import androidx.appcompat.app.AppCompatActivity
import tech.magichacker.myrtcdemo.databinding.ActivityMainBinding

class MainActivity : AppCompatActivity() {
    private lateinit var binding: ActivityMainBinding

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        binding = ActivityMainBinding.inflate(layoutInflater)
        setContentView(binding.root)
        if (checkSelfPermission(android.Manifest.permission.RECORD_AUDIO) != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(
                arrayOf(
                    android.Manifest.permission.RECORD_AUDIO
                ), 100
            )
            return
        }
        binding.normal.setOnClickListener {
            startActivity(Intent(this, NormalModeActivity::class.java))
        }
        binding.websocket.setOnClickListener {
            startActivity(Intent(this, WebSocketModeActivity::class.java))
        }
    }

    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (resultCode == Activity.RESULT_OK && requestCode == 100) {
            finish()
            startActivity(Intent(this@MainActivity, MainActivity::class.java))
        }
    }
}