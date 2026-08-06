package uz.loadify.core.ui

import android.app.Activity
import android.os.Build
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.ColorScheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.SideEffect
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.unit.dp
import androidx.core.view.WindowCompat

/**
 * Sampled from the logo. Kept in step with res/values/colors.xml, which is what
 * the launcher icon reads — the two must not drift, or the app opens in a
 * different blue than the icon the driver tapped.
 */
private val Brand = Color(0xFF1E56C9)
private val Accent = Color(0xFF2FAE4F)
private val Navy = Color(0xFF14275C)

private val Warning = Color(0xFFF59E0B)
private val Danger = Color(0xFFDC2626)

/**
 * The palette is deliberately high-contrast: this app is used in a cab, in
 * daylight, often through a windscreen reflection.
 */
private val LightColors = lightColorScheme(
    primary = Brand,
    onPrimary = Color.White,
    secondary = Accent,
    onSecondary = Color.White,
    tertiary = Warning,
    error = Danger,
    background = Color(0xFFF6F7F9),
    surface = Color.White,
    // The wordmark's navy, used for headings — it is the darkest brand value
    // and reads better than pure black against the blue.
    onSurface = Navy,
    surfaceContainerHighest = Color(0xFFE9ECF2),
    outlineVariant = Color(0xFFDDE1E9),
)

private val DarkColors = darkColorScheme(
    primary = Color(0xFF8AA4FF),
    onPrimary = Color(0xFF00164C),
    secondary = Color(0xFF4FD6A0),
    onSecondary = Color(0xFF00301E),
    tertiary = Warning,
    error = Color(0xFFFF8A80),
    background = Color(0xFF0E1116),
    surface = Color(0xFF161B23),
    surfaceContainerHighest = Color(0xFF232A35),
    outlineVariant = Color(0xFF2E3743),
)

private val LoadifyShapes = Shapes(
    small = RoundedCornerShape(8.dp),
    medium = RoundedCornerShape(12.dp),
    large = RoundedCornerShape(14.dp),
)

@Composable
fun LoadifyTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    // Off by default: the brand blue is load-bearing here, and a wallpaper-tinted
    // status badge would let the palette drift device by device.
    dynamicColor: Boolean = false,
    content: @Composable () -> Unit,
) {
    val colors = when {
        dynamicColor && Build.VERSION.SDK_INT >= Build.VERSION_CODES.S -> {
            val context = LocalContext.current
            if (darkTheme) {
                androidx.compose.material3.dynamicDarkColorScheme(context)
            } else {
                androidx.compose.material3.dynamicLightColorScheme(context)
            }
        }
        darkTheme -> DarkColors
        else -> LightColors
    }

    val view = LocalView.current
    if (!view.isInEditMode) {
        SideEffect {
            val window = (view.context as Activity).window
            WindowCompat.getInsetsController(window, view)
                .isAppearanceLightStatusBars = !darkTheme
        }
    }

    MaterialTheme(colorScheme = colors, shapes = LoadifyShapes, content = content)
}

/**
 * Status colours, kept as one mapping so a badge and a progress rail cannot
 * disagree about what "delivered" looks like.
 */
fun statusColor(status: String, scheme: ColorScheme): Color = when (status) {
    "published", "online", "accepted", "completed", "delivered" -> Accent
    "pending", "offered", "reserved", "busy", "in_transit" -> Warning
    "cancelled", "rejected", "expired", "disputed" -> Danger
    else -> scheme.onSurfaceVariant
}
