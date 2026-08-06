package uz.loadify

import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.BookmarkBorder
import androidx.compose.material.icons.outlined.LocalShipping
import androidx.compose.material.icons.outlined.Person
import androidx.compose.material.icons.outlined.Route
import androidx.compose.material.icons.automirrored.rounded.Login
import androidx.compose.material.icons.rounded.BookmarkBorder
import androidx.compose.material.icons.rounded.LocalShipping
import androidx.compose.material.icons.rounded.Person
import androidx.compose.material.icons.rounded.Route
import androidx.compose.material3.Icon
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavDestination.Companion.hierarchy
import androidx.navigation.NavGraph.Companion.findStartDestination
import androidx.navigation.NavHostController
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import uz.loadify.core.format.Fmt
import uz.loadify.feature.auth.ui.LoginScreen
import uz.loadify.feature.auth.ui.SessionViewModel
import uz.loadify.feature.bookings.ui.BookingsScreen
import uz.loadify.feature.driver.ui.ProfileScreen
import uz.loadify.feature.loads.ui.BoardScreen
import uz.loadify.feature.loads.ui.LoadDetailScreen
import uz.loadify.feature.trips.data.TripRepository
import uz.loadify.feature.trips.ui.TripsScreen

private object Routes {
    const val BOARD = "board"
    const val BOOKINGS = "bookings"
    const val TRIPS = "trips"
    const val PROFILE = "profile"
    const val LOGIN = "login"
    const val LOAD_DETAIL = "loads/{loadId}"

    fun loadDetail(id: String) = "loads/$id"
}

private data class Tab(
    val route: String,
    val labelRes: Int,
    val selectedIcon: ImageVector,
    val icon: ImageVector,
)

@Composable
fun LoadifyApp(
    tripRepository: TripRepository,
    onLocaleChange: (String) -> Unit,
    sessionViewModel: SessionViewModel = hiltViewModel(),
) {
    val navController = rememberNavController()
    val user by sessionViewModel.user.collectAsStateWithLifecycle()
    val locale by sessionViewModel.locale.collectAsStateWithLifecycle()
    val fmt = remember(locale) { Fmt(locale) }

    val signedIn = user != null

    val tabs = listOf(
        Tab(Routes.BOARD, R.string.nav_board, Icons.Rounded.LocalShipping, Icons.Outlined.LocalShipping),
        Tab(Routes.BOOKINGS, R.string.nav_bookings, Icons.Rounded.BookmarkBorder, Icons.Outlined.BookmarkBorder),
        Tab(Routes.TRIPS, R.string.nav_trips, Icons.Rounded.Route, Icons.Outlined.Route),
        Tab(Routes.PROFILE, R.string.nav_profile, Icons.Rounded.Person, Icons.Outlined.Person),
    )

    val backStack by navController.currentBackStackEntryAsState()
    val currentRoute = backStack?.destination
    // The bottom bar is hidden on pushed screens so a load detail gets the full
    // height for the price and the reserve button.
    val showBar = tabs.any { tab ->
        currentRoute?.hierarchy?.any { it.route == tab.route } == true
    }

    Scaffold(
        bottomBar = {
            if (showBar) {
                NavigationBar {
                    tabs.forEach { tab ->
                        val selected =
                            currentRoute?.hierarchy?.any { it.route == tab.route } == true
                        NavigationBarItem(
                            selected = selected,
                            onClick = { navController.switchTab(tab.route) },
                            icon = {
                                Icon(
                                    when {
                                        tab.route == Routes.PROFILE && !signedIn ->
                                            Icons.AutoMirrored.Rounded.Login
                                        selected -> tab.selectedIcon
                                        else -> tab.icon
                                    },
                                    contentDescription = null,
                                )
                            },
                            label = { Text(stringResource(tab.labelRes)) },
                        )
                    }
                }
            }
        },
    ) { padding ->
        NavHost(
            navController = navController,
            startDestination = Routes.BOARD,
            modifier = Modifier.padding(padding),
        ) {
            composable(Routes.BOARD) {
                BoardScreen(
                    user = user,
                    fmt = fmt,
                    onOpenLoad = { navController.navigate(Routes.loadDetail(it)) },
                )
            }

            composable(Routes.LOAD_DETAIL) { entry ->
                LoadDetailScreen(
                    loadId = entry.arguments?.getString("loadId").orEmpty(),
                    user = user,
                    fmt = fmt,
                    onBack = { navController.popBackStack() },
                )
            }

            // The board is public — a driver evaluating the app sees real
            // freight before signing up — so only these three gate on a session.
            composable(Routes.BOOKINGS) {
                IfSignedIn(signedIn, navController) { BookingsScreen(user = user, fmt = fmt) }
            }

            composable(Routes.TRIPS) {
                IfSignedIn(signedIn, navController) {
                    TripsScreen(fmt = fmt, tripRepository = tripRepository)
                }
            }

            composable(Routes.PROFILE) {
                IfSignedIn(signedIn, navController) {
                    ProfileScreen(
                        user = user,
                        fmt = fmt,
                        currentLocale = locale,
                        onLocaleChange = {
                            sessionViewModel.setLocale(it)
                            onLocaleChange(it)
                        },
                        onSignOut = sessionViewModel::signOut,
                    )
                }
            }

            composable(Routes.LOGIN) {
                LoginScreen(
                    onSignedIn = { navController.popBackStack() },
                    currentLocale = locale,
                    onLocaleChange = {
                        sessionViewModel.setLocale(it)
                        onLocaleChange(it)
                    },
                )
            }
        }
    }
}

/**
 * Sends an unauthenticated visitor to login instead of rendering an
 * authenticated tab, and brings them straight back once signed in.
 */
@Composable
private fun IfSignedIn(
    signedIn: Boolean,
    navController: NavHostController,
    content: @Composable () -> Unit,
) {
    if (signedIn) {
        content()
    } else {
        androidx.compose.runtime.LaunchedEffect(Unit) {
            navController.navigate(Routes.LOGIN)
        }
    }
}

/**
 * Standard bottom-bar behaviour: one entry per tab on the back stack, state
 * preserved, and tapping the active tab returns it to its root.
 */
private fun NavHostController.switchTab(route: String) {
    navigate(route) {
        popUpTo(graph.findStartDestination().id) { saveState = true }
        launchSingleTop = true
        restoreState = true
    }
}
