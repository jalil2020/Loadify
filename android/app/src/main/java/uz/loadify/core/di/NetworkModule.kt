package uz.loadify.core.di

import android.content.Context
import com.jakewharton.retrofit2.converter.kotlinx.serialization.asConverterFactory
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import retrofit2.Retrofit
import uz.loadify.BuildConfig
import uz.loadify.core.network.ApiCall
import uz.loadify.core.network.AuthInterceptor
import uz.loadify.core.network.Envelope
import uz.loadify.core.network.LoadifyApi
import uz.loadify.core.network.TokenAuthenticator
import uz.loadify.core.network.TokenRefresher
import uz.loadify.core.storage.Session
import uz.loadify.core.storage.SessionStore
import uz.loadify.feature.auth.domain.AuthResponse
import java.io.IOException
import java.util.concurrent.TimeUnit
import javax.inject.Singleton

@Module
@InstallIn(SingletonComponent::class)
object NetworkModule {

    @Provides
    @Singleton
    fun json(): Json = Json {
        // The server adds fields ahead of the client; failing on an unknown one
        // would break every old install the moment a field ships.
        ignoreUnknownKeys = true
        coerceInputValues = true
        explicitNulls = false
    }

    @Provides
    @Singleton
    fun sessionStore(@ApplicationContext context: Context, json: Json): SessionStore =
        SessionStore(context, json)

    @Provides
    @Singleton
    fun apiCall(json: Json): ApiCall = ApiCall(json)

    /**
     * The refresh call, made on a bare client.
     *
     * It deliberately does not go through the authenticated OkHttp instance: a
     * refresh that itself 401s must not re-enter the authenticator that is
     * currently holding the lock and waiting on this call.
     */
    @Provides
    @Singleton
    fun tokenRefresher(json: Json): TokenRefresher = TokenRefresher { refreshToken ->
        val client = OkHttpClient.Builder()
            .connectTimeout(15, TimeUnit.SECONDS)
            .readTimeout(20, TimeUnit.SECONDS)
            .build()

        val body = """{"refresh_token":"$refreshToken"}"""
            .toRequestBody("application/json".toMediaType())
        val request = Request.Builder()
            .url("${BuildConfig.API_URL.trimEnd('/')}/auth/refresh")
            .post(body)
            .build()

        try {
            client.newCall(request).execute().use { response ->
                if (!response.isSuccessful) return@TokenRefresher null
                val raw = response.body?.string() ?: return@TokenRefresher null
                val envelope = json.decodeFromString(
                    Envelope.serializer(AuthResponse.serializer()),
                    raw,
                )
                val auth = envelope.data
                // The tokens are nested under `tokens`. A payload without both
                // is rejected rather than stored: an empty access token is sent
                // as no header at all, which looks like a broken app rather
                // than an expired session.
                if (auth == null || !auth.isUsable) return@TokenRefresher null
                Session(
                    accessToken = auth.tokens!!.accessToken,
                    refreshToken = auth.tokens.refreshToken,
                    expiresAt = auth.tokens.expiresAt,
                    // Roles and permissions can change while a session is open,
                    // so the refreshed principal wins over the stored copy.
                    user = auth.user!!,
                )
            }
        } catch (_: IOException) {
            // A refresh that failed on the network is not proof the token is
            // bad. Returning null without clearing lets the next request try
            // again, rather than signing a driver out for a tunnel.
            null
        } catch (_: IllegalArgumentException) {
            null
        }
    }

    @Provides
    @Singleton
    fun okHttpClient(
        sessions: SessionStore,
        refresher: TokenRefresher,
    ): OkHttpClient = OkHttpClient.Builder()
        .addInterceptor(AuthInterceptor(sessions))
        .authenticator(TokenAuthenticator(sessions, refresher))
        .connectTimeout(15, TimeUnit.SECONDS)
        // Generous: a driver on a rural 3G cell is the target user, and a
        // spurious timeout mid-reservation loses them the load.
        .readTimeout(30, TimeUnit.SECONDS)
        .retryOnConnectionFailure(true)
        .build()

    @Provides
    @Singleton
    fun retrofit(client: OkHttpClient, json: Json): Retrofit = Retrofit.Builder()
        .baseUrl(BuildConfig.API_URL.trimEnd('/') + "/")
        .client(client)
        .addConverterFactory(json.asConverterFactory("application/json".toMediaType()))
        .build()

    @Provides
    @Singleton
    fun loadifyApi(retrofit: Retrofit): LoadifyApi = retrofit.create(LoadifyApi::class.java)
}
