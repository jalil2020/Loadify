/// A failed request, carrying the backend's stable machine code.
///
/// Callers branch on [code], never on [message] — messages are localised and
/// change, codes are part of the contract.
class ApiException implements Exception {
  const ApiException({
    required this.code,
    required this.message,
    required this.status,
    this.fieldErrors = const {},
    this.traceId,
  });

  final String code;
  final String message;
  final int status;

  /// field name -> message, ready to drop onto form inputs.
  final Map<String, String> fieldErrors;

  /// Quote this when reporting a problem; it ties the phone to a server log.
  final String? traceId;

  bool get isValidation => code == 'VALIDATION_ERROR';
  bool get isAuth => status == 401;

  /// The whole token family was revoked as a theft response. Retrying is
  /// futile — the only correct move is to sign in again.
  bool get isTokenReuse => code == 'TOKEN_REUSE_DETECTED';

  /// Worth offering a retry button for; a 4xx generally is not.
  bool get isTransient => status >= 500 || status == 0 || status == 429;

  factory ApiException.network(Object cause) => ApiException(
    code: 'NETWORK_ERROR',
    message: 'Could not reach Loadify. Check your connection. ($cause)',
    status: 0,
  );

  /// Parses the error envelope. Anything unrecognisable becomes a generic
  /// failure rather than throwing while handling a throw.
  factory ApiException.fromEnvelope(Object? body, int status) {
    if (body is! Map) {
      return ApiException(
        code: 'UNKNOWN',
        message: 'Something went wrong',
        status: status,
      );
    }
    final Object? error = body['error'];
    if (error is! Map) {
      return ApiException(
        code: 'UNKNOWN',
        message: 'Something went wrong',
        status: status,
      );
    }

    final Map<String, String> fields = <String, String>{};
    final Object? details = error['details'];
    if (details is List) {
      for (final Object? entry in details) {
        if (entry is Map) {
          final Object? field = entry['field'];
          final Object? message = entry['message'];
          if (field is String && message is String) {
            fields[field] = message;
          }
        }
      }
    }

    return ApiException(
      code: error['code'] as String? ?? 'UNKNOWN',
      message: error['message'] as String? ?? 'Something went wrong',
      status: status,
      fieldErrors: fields,
      traceId: body['trace_id'] as String?,
    );
  }

  @override
  String toString() => 'ApiException($code/$status): $message';
}
