package uz.loadify.core.ui

/**
 * The languages the product ships.
 *
 * Their labels are written in the language itself, never translated: someone
 * looking for Russian is looking for the word "Русский", not for whatever the
 * current UI language calls it.
 */
val SUPPORTED_LOCALES = listOf("uz", "ru", "en")

fun localeLabel(tag: String): String = when (tag) {
    "uz" -> "O'zbekcha"
    "ru" -> "Русский"
    "en" -> "English"
    else -> tag
}
