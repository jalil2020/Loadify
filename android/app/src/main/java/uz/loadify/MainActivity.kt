package uz.loadify

import android.os.Bundle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.appcompat.app.AppCompatActivity
import androidx.appcompat.app.AppCompatDelegate
import androidx.core.os.LocaleListCompat
import dagger.hilt.android.AndroidEntryPoint
import uz.loadify.core.storage.SessionStore
import uz.loadify.core.ui.LoadifyTheme
import uz.loadify.feature.trips.data.TripRepository
import javax.inject.Inject

/**
 * AppCompatActivity, not ComponentActivity, purely so AppCompatDelegate can
 * apply a per-app language on devices below API 33. Everything drawn is Compose.
 */
@AndroidEntryPoint
class MainActivity : AppCompatActivity() {

    @Inject lateinit var sessions: SessionStore

    @Inject lateinit var trips: TripRepository

    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)

        // Applied before the first frame so the app opens in the driver's
        // language rather than flashing the system one and correcting itself.
        applyLocale(sessions.currentLocale)

        setContent {
            LoadifyTheme {
                LoadifyApp(
                    tripRepository = trips,
                    onLocaleChange = ::applyLocale,
                )
            }
        }
    }

    private fun applyLocale(tag: String) {
        val current = AppCompatDelegate.getApplicationLocales()
        // Setting the same locale recreates the activity for nothing, which on
        // a slow device is a visible flicker every time the screen recomposes.
        if (current.toLanguageTags() == tag) return
        AppCompatDelegate.setApplicationLocales(LocaleListCompat.forLanguageTags(tag))
    }
}
