package uz.loadify.feature.loads.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.LocalShipping
import androidx.compose.material.icons.rounded.Tune
import androidx.compose.material3.Badge
import androidx.compose.material3.BadgedBox
import androidx.compose.material3.Button
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarDuration
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import uz.loadify.R
import uz.loadify.core.format.Fmt
import uz.loadify.core.ui.EmptyView
import uz.loadify.core.ui.ErrorView
import uz.loadify.core.ui.SkeletonList
import uz.loadify.core.ui.UiState
import uz.loadify.feature.auth.domain.CurrentUser
import uz.loadify.feature.loads.domain.LoadFilters

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun BoardScreen(
    user: CurrentUser?,
    fmt: Fmt,
    onOpenLoad: (String) -> Unit,
    viewModel: BoardViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val message by viewModel.messages.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }
    var showFilters by remember { mutableStateOf(false) }

    val reservedText = stringResource(R.string.board_reserved)
    val genericError = stringResource(R.string.common_error)
    val canReserve = user?.can("bookings.create") == true

    LaunchedEffect(message) {
        message?.let {
            snackbar.showSnackbar(it.text, duration = SnackbarDuration.Short)
            viewModel.consumeMessage()
        }
    }

    Scaffold(
        snackbarHost = { SnackbarHost(snackbar) },
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.board_title)) },
                actions = {
                    IconButton(onClick = { showFilters = true }) {
                        BadgedBox(
                            badge = { if (!state.filters.isEmpty) Badge() },
                        ) {
                            Icon(
                                Icons.Rounded.Tune,
                                contentDescription = stringResource(R.string.filter_title),
                            )
                        }
                    }
                },
            )
        },
    ) { padding ->
        PullToRefreshBox(
            isRefreshing = state.refreshing,
            onRefresh = viewModel::refresh,
            modifier = Modifier
                .fillMaxSize()
                .padding(padding),
        ) {
            when (val loads = state.loads) {
                is UiState.Loading -> SkeletonList()

                is UiState.Failure -> ErrorView(loads.error, onRetry = viewModel::refresh)

                is UiState.Success -> {
                    if (loads.value.items.isEmpty()) {
                        EmptyView(
                            title = stringResource(R.string.board_empty),
                            hint = stringResource(R.string.board_empty_hint),
                            icon = Icons.Outlined.LocalShipping,
                        )
                    } else {
                        LazyColumn(
                            contentPadding = PaddingValues(16.dp),
                            verticalArrangement = Arrangement.spacedBy(12.dp),
                            modifier = Modifier.fillMaxSize(),
                        ) {
                            item {
                                Text(
                                    stringResource(
                                        R.string.board_loads_found,
                                        fmt.number(loads.value.meta.total),
                                    ),
                                    style = MaterialTheme.typography.bodySmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                            }
                            items(loads.value.items, key = { it.id }) { load ->
                                LoadCard(
                                    load = load,
                                    fmt = fmt,
                                    onClick = { onOpenLoad(load.id) },
                                    onReserve = if (canReserve) {
                                        {
                                            viewModel.reserve(
                                                load,
                                                "${load.referenceNo}: $reservedText",
                                                genericError,
                                            )
                                        }
                                    } else {
                                        null
                                    },
                                    reserving = state.reservingId == load.id,
                                )
                            }
                        }
                    }
                }
            }
        }
    }

    if (showFilters) {
        val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
        ModalBottomSheet(
            onDismissRequest = { showFilters = false },
            sheetState = sheetState,
        ) {
            FilterSheet(
                state = state,
                isDriver = user?.isDriver == true,
                onChange = viewModel::updateFilters,
                onClear = viewModel::clearFilters,
                onApply = { showFilters = false },
            )
        }
    }
}

/**
 * Filters live in a sheet rather than above the list: on a phone the board is
 * the scarce resource, and a driver changes filters far less often than they
 * scroll.
 */
@Composable
private fun FilterSheet(
    state: BoardUiState,
    isDriver: Boolean,
    onChange: ((LoadFilters) -> LoadFilters) -> Unit,
    onClear: () -> Unit,
    onApply: () -> Unit,
) {
    Column(
        Modifier
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 16.dp)
            .padding(bottom = 32.dp),
    ) {
        Text(
            stringResource(R.string.filter_title),
            style = MaterialTheme.typography.titleLarge,
        )
        Spacer(Modifier.height(16.dp))

        val regionOptions = state.regions.map { it.id to it.name }
        val vehicleOptions = state.vehicleTypes.map { it.code to it.name }

        PickerField(
            label = stringResource(R.string.filter_from),
            placeholder = stringResource(R.string.filter_any_region),
            options = regionOptions,
            selected = state.filters.pickupRegionId,
            onSelect = { id -> onChange { it.copy(pickupRegionId = id) } },
        )
        Spacer(Modifier.height(12.dp))

        PickerField(
            label = stringResource(R.string.filter_to),
            placeholder = stringResource(R.string.filter_any_region),
            options = regionOptions,
            selected = state.filters.deliveryRegionId,
            onSelect = { id -> onChange { it.copy(deliveryRegionId = id) } },
        )
        Spacer(Modifier.height(12.dp))

        PickerField(
            label = stringResource(R.string.filter_vehicle_type),
            placeholder = stringResource(R.string.filter_any_vehicle),
            options = vehicleOptions,
            selected = state.filters.vehicleType,
            onSelect = { code -> onChange { it.copy(vehicleType = code) } },
        )
        Spacer(Modifier.height(12.dp))

        Row {
            WeightField(
                label = stringResource(R.string.filter_min_weight),
                value = state.filters.minWeightKg,
                onChange = { kg -> onChange { it.copy(minWeightKg = kg) } },
                modifier = Modifier.weight(1f),
            )
            Spacer(Modifier.width(12.dp))
            WeightField(
                label = stringResource(R.string.filter_max_weight),
                value = state.filters.maxWeightKg,
                onChange = { kg -> onChange { it.copy(maxWeightKg = kg) } },
                modifier = Modifier.weight(1f),
            )
        }

        // Only a driver has a vehicle to match against, so the switch is absent
        // rather than disabled for everyone else.
        if (isDriver) {
            Spacer(Modifier.height(8.dp))
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    stringResource(R.string.filter_fits_my_vehicle),
                    style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier.weight(1f),
                )
                Switch(
                    checked = state.filters.fitsMyVehicle,
                    onCheckedChange = { on -> onChange { it.copy(fitsMyVehicle = on) } },
                )
            }
        }

        Spacer(Modifier.height(16.dp))
        Row {
            OutlinedButton(onClick = onClear, modifier = Modifier.weight(1f)) {
                Text(stringResource(R.string.filter_clear))
            }
            Spacer(Modifier.width(12.dp))
            Button(onClick = onApply, modifier = Modifier.weight(1f)) {
                Text(stringResource(R.string.filter_apply))
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun PickerField(
    label: String,
    placeholder: String,
    options: List<Pair<String, String>>,
    selected: String?,
    onSelect: (String?) -> Unit,
) {
    var expanded by remember { mutableStateOf(false) }
    val selectedLabel = options.firstOrNull { it.first == selected }?.second ?: placeholder

    ExposedDropdownMenuBox(
        expanded = expanded,
        onExpandedChange = { expanded = it },
    ) {
        OutlinedTextField(
            value = selectedLabel,
            onValueChange = {},
            readOnly = true,
            label = { Text(label) },
            trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded) },
            modifier = Modifier
                .menuAnchor(androidx.compose.material3.MenuAnchorType.PrimaryNotEditable)
                .fillMaxWidth(),
        )
        ExposedDropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            DropdownMenuItem(
                text = { Text(placeholder) },
                onClick = {
                    onSelect(null)
                    expanded = false
                },
            )
            options.forEach { (id, name) ->
                DropdownMenuItem(
                    text = { Text(name) },
                    onClick = {
                        onSelect(id)
                        expanded = false
                    },
                )
            }
        }
    }
}

@Composable
private fun WeightField(
    label: String,
    value: Long?,
    onChange: (Long?) -> Unit,
    modifier: Modifier = Modifier,
) {
    OutlinedTextField(
        value = value?.toString().orEmpty(),
        onValueChange = { text -> onChange(text.takeIf { it.isNotEmpty() }?.toLongOrNull()) },
        label = { Text(label) },
        singleLine = true,
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
        modifier = modifier,
    )
}
