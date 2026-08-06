package uz.loadify.core.storage

/**
 * The slice of the session the network layer actually needs.
 *
 * The interceptor and the authenticator depend on this rather than on
 * [SessionStore] so they can be exercised without a keystore — the real store
 * needs an Android context, and a refresh race is exactly the thing worth
 * testing on the JVM rather than on a device.
 */
interface SessionHolder {
    val current: Session?
    val currentLocale: String
    fun save(session: Session)
    fun clear()
}
