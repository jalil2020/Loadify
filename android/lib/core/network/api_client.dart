import 'dart:async';

import 'package:dio/dio.dart';

import '../config/env.dart';
import '../error/api_exception.dart';
import '../storage/session_store.dart';
import '../../features/auth/domain/auth_response.dart';

/// The response envelope every endpoint returns.
class Paged<T> {
  const Paged({required this.items, required this.meta});

  final List<T> items;
  final PageMeta meta;
}

class PageMeta {
  const PageMeta({
    required this.page,
    required this.perPage,
    required this.total,
    required this.totalPages,
  });

  final int page;
  final int perPage;
  final int total;
  final int totalPages;

  bool get hasMore => page < totalPages;

  factory PageMeta.fromJson(Map<String, dynamic>? json) {
    if (json == null) {
      return const PageMeta(page: 1, perPage: 0, total: 0, totalPages: 1);
    }
    return PageMeta(
      page: (json['page'] as num?)?.toInt() ?? 1,
      perPage: (json['per_page'] as num?)?.toInt() ?? 0,
      total: (json['total'] as num?)?.toInt() ?? 0,
      totalPages: (json['total_pages'] as num?)?.toInt() ?? 1,
    );
  }
}

/// Raised when the session is gone for good and the UI must return to login.
class SessionExpired implements Exception {
  const SessionExpired();
}

/// The HTTP client.
///
/// Owns the session because token refresh is a property of the transport, not
/// of any one screen: a driver mid-trip should never see a login prompt just
/// because a fifteen-minute access token lapsed while the app was backgrounded.
class ApiClient {
  ApiClient(this._store, {Dio? dio, String? baseUrl})
    : _dio =
          dio ??
          Dio(
            BaseOptions(
              baseUrl: baseUrl ?? Env.apiBaseUrl,
              connectTimeout: const Duration(seconds: 15),
              // Generous: a driver on a rural 3G cell is the target user, and
              // a spurious timeout mid-reservation loses them the load.
              receiveTimeout: const Duration(seconds: 30),
              // Non-2xx is handled by us, not thrown by Dio, so the envelope
              // can be parsed into a typed ApiException.
              validateStatus: (_) => true,
            ),
          ) {
    _dio.interceptors.add(
      InterceptorsWrapper(
        onRequest: (RequestOptions options, RequestInterceptorHandler handler) {
          options.headers['Accept-Language'] = _store.locale;
          final String? token = _session?.accessToken;
          if (token != null && options.extra['anonymous'] != true) {
            options.headers['Authorization'] = 'Bearer $token';
          }
          handler.next(options);
        },
      ),
    );
  }

  final Dio _dio;
  final SessionStore _store;

  Session? _session;
  Session? get session => _session;

  final StreamController<Session?> _sessions =
      StreamController<Session?>.broadcast();

  /// Emits on sign-in, refresh and sign-out so the router can react.
  Stream<Session?> get sessionChanges => _sessions.stream;

  Future<void> restore() async {
    _session = await _store.read();
    _sessions.add(_session);
  }

  Future<void> adopt(Session session) async {
    _session = session;
    await _store.write(session);
    _sessions.add(session);
  }

  Future<void> signOut() async {
    _session = null;
    await _store.clear();
    _sessions.add(null);
  }

  /// Concurrent 401s share one refresh.
  ///
  /// Without this, a dashboard opening five queries at once fires five refresh
  /// requests; the backend rotates the token on first use, so the other four
  /// present an already-rotated token and trip its theft detection — signing
  /// the driver out for doing nothing wrong.
  Future<bool>? _refreshInFlight;

  Future<bool> _ensureRefresh() {
    return _refreshInFlight ??= _refresh().whenComplete(() {
      _refreshInFlight = null;
    });
  }

  Future<bool> _refresh() async {
    final String? refreshToken = _session?.refreshToken;
    if (refreshToken == null) return false;

    late final Response<dynamic> res;
    try {
      res = await _dio.post<dynamic>(
        '/auth/refresh',
        data: <String, dynamic>{'refresh_token': refreshToken},
        options: Options(extra: <String, dynamic>{'anonymous': true}),
      );
    } on DioException {
      // A refresh that failed on the network is not proof the token is bad, so
      // the session is kept and the next request will try again. Clearing here
      // would sign a driver out every time they drove through a tunnel.
      return false;
    }

    if (res.statusCode != 200) {
      await signOut();
      return false;
    }

    final Object? body = res.data;
    if (body is! Map<String, dynamic>) {
      await signOut();
      return false;
    }

    // The tokens are nested under `tokens`, not spread at the top level.
    // Reading `data.access_token` here yields null, and every later request
    // goes out unauthenticated — a failure that looks like a broken app rather
    // than an expired session.
    final AuthResponse? auth = AuthResponse.tryParse(body['data']);
    if (auth == null) {
      await signOut();
      return false;
    }

    await adopt(
      Session(
        accessToken: auth.tokens.accessToken,
        refreshToken: auth.tokens.refreshToken,
        expiresAt: auth.tokens.expiresAt,
        // Roles and permissions can change while a session is open, so the
        // refreshed principal wins over the copy held since sign-in.
        user: auth.user,
      ),
    );
    return true;
  }

  Future<Response<dynamic>> _send(
    String method,
    String path, {
    Object? body,
    Map<String, dynamic>? query,
    bool anonymous = false,
    bool retried = false,
  }) async {
    final Response<dynamic> res;
    try {
      res = await _dio.request<dynamic>(
        path,
        data: body,
        queryParameters: _clean(query),
        options: Options(
          method: method,
          extra: <String, dynamic>{'anonymous': anonymous},
        ),
      );
    } on DioException catch (e) {
      throw ApiException.network(e.message ?? e.type.name);
    }

    final int status = res.statusCode ?? 0;
    if (status >= 200 && status < 300) return res;

    final ApiException error = ApiException.fromEnvelope(res.data, status);

    if (status == 401 && !retried && !anonymous && !error.isTokenReuse) {
      if (await _ensureRefresh()) {
        return _send(
          method,
          path,
          body: body,
          query: query,
          anonymous: anonymous,
          retried: true,
        );
      }
      throw const SessionExpired();
    }
    if (error.isTokenReuse) {
      await signOut();
      throw const SessionExpired();
    }
    throw error;
  }

  /// Drops empty parameters and expands lists into repeated keys, which is how
  /// the API takes multi-value filters: `?vehicle_type=tent&vehicle_type=van`.
  Map<String, dynamic>? _clean(Map<String, dynamic>? query) {
    if (query == null) return null;
    final Map<String, dynamic> out = <String, dynamic>{};
    query.forEach((String key, Object? value) {
      if (value == null || value == '') return;
      if (value is List) {
        final List<Object?> kept = value
            .where((Object? v) => v != null && v != '')
            .toList();
        if (kept.isNotEmpty) out[key] = kept;
        return;
      }
      out[key] = value;
    });
    return out;
  }

  Map<String, dynamic> _envelope(Response<dynamic> res) {
    final Object? body = res.data;
    if (body is Map<String, dynamic>) return body;
    throw ApiException(
      code: 'MALFORMED_RESPONSE',
      message: 'The server returned an unreadable response',
      status: res.statusCode ?? 0,
    );
  }

  /// A single object.
  Future<T> get<T>(
    String path,
    T Function(Map<String, dynamic>) parse, {
    Map<String, dynamic>? query,
    bool anonymous = false,
  }) async {
    final Response<dynamic> res = await _send(
      'GET',
      path,
      query: query,
      anonymous: anonymous,
    );
    final Object? data = _envelope(res)['data'];
    if (data is! Map<String, dynamic>) {
      throw ApiException(
        code: 'MALFORMED_RESPONSE',
        message: 'Expected an object at $path',
        status: res.statusCode ?? 0,
      );
    }
    return parse(data);
  }

  /// A paginated list.
  Future<Paged<T>> getPage<T>(
    String path,
    T Function(Map<String, dynamic>) parse, {
    Map<String, dynamic>? query,
    bool anonymous = false,
  }) async {
    final Response<dynamic> res = await _send(
      'GET',
      path,
      query: query,
      anonymous: anonymous,
    );
    final Map<String, dynamic> envelope = _envelope(res);
    return Paged<T>(
      items: _list(envelope['data'], parse),
      meta: PageMeta.fromJson(envelope['meta'] as Map<String, dynamic>?),
    );
  }

  /// A bare list, for reference vocabularies that are not paginated.
  Future<List<T>> getList<T>(
    String path,
    T Function(Map<String, dynamic>) parse, {
    Map<String, dynamic>? query,
    bool anonymous = false,
  }) async {
    final Response<dynamic> res = await _send(
      'GET',
      path,
      query: query,
      anonymous: anonymous,
    );
    return _list(_envelope(res)['data'], parse);
  }

  List<T> _list<T>(Object? data, T Function(Map<String, dynamic>) parse) {
    if (data is! List) return <T>[];
    return data
        .whereType<Map<String, dynamic>>()
        .map<T>(parse)
        .toList(growable: false);
  }

  Future<T> post<T>(
    String path,
    T Function(Map<String, dynamic>) parse, {
    Object? body,
    bool anonymous = false,
  }) async {
    final Response<dynamic> res = await _send(
      'POST',
      path,
      body: body,
      anonymous: anonymous,
    );
    final Object? data = _envelope(res)['data'];
    if (data is! Map<String, dynamic>) {
      throw ApiException(
        code: 'MALFORMED_RESPONSE',
        message: 'Expected an object at $path',
        status: res.statusCode ?? 0,
      );
    }
    return parse(data);
  }

  /// For endpoints whose result is the status code, not a body.
  Future<void> send(
    String method,
    String path, {
    Object? body,
    bool anonymous = false,
  }) async {
    await _send(method, path, body: body, anonymous: anonymous);
  }

  void dispose() {
    unawaited(_sessions.close());
    _dio.close(force: true);
  }
}
