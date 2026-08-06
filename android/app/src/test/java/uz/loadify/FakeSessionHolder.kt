package uz.loadify

import uz.loadify.core.storage.Session
import uz.loadify.core.storage.SessionHolder
import uz.loadify.feature.auth.domain.CurrentUser
import java.util.concurrent.atomic.AtomicInteger

/** An in-memory session, so the network layer can be tested without a keystore. */
class FakeSessionHolder(initial: Session? = null) : SessionHolder {

    @Volatile
    private var session: Session? = initial

    val saveCount = AtomicInteger(0)
    var cleared: Boolean = false
        private set

    override val current: Session? get() = session
    override val currentLocale: String = "uz"

    override fun save(session: Session) {
        this.session = session
        saveCount.incrementAndGet()
    }

    override fun clear() {
        session = null
        cleared = true
    }

    companion object {
        fun session(accessToken: String, refreshToken: String = "refresh-1") = Session(
            accessToken = accessToken,
            refreshToken = refreshToken,
            expiresAt = "2030-01-01T00:00:00Z",
            user = CurrentUser(id = "u1", phone = "+998901111111", fullName = "Anvar Tursunov"),
        )
    }
}
