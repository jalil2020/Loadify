/// The signed-in principal.
class CurrentUser {
  const CurrentUser({
    required this.id,
    required this.phone,
    required this.firstName,
    required this.lastName,
    required this.fullName,
    required this.locale,
    required this.status,
    required this.phoneVerified,
    required this.roles,
    required this.permissions,
    this.email,
    this.avatarUrl,
    this.driverId,
    this.companyId,
    this.companyRole,
  });

  final String id;
  final String phone;
  final String firstName;
  final String lastName;
  final String fullName;
  final String locale;
  final String status;
  final bool phoneVerified;
  final List<String> roles;

  /// The flattened effective set across every role. Gate UI on a permission,
  /// never on a role name, so adding a role does not require an app release —
  /// which on mobile means waiting on a store review.
  final List<String> permissions;

  final String? email;
  final String? avatarUrl;
  final String? driverId;
  final String? companyId;
  final String? companyRole;

  bool can(String permission) => permissions.contains(permission);

  bool get isDriver => driverId != null && driverId!.isNotEmpty;

  String get initials {
    final List<String> parts = fullName
        .split(RegExp(r'\s+'))
        .where((String p) => p.isNotEmpty)
        .toList();
    if (parts.isEmpty) return '?';
    return parts.take(2).map((String p) => p[0].toUpperCase()).join();
  }

  factory CurrentUser.fromJson(Map<String, dynamic> json) => CurrentUser(
    id: json['id'] as String? ?? '',
    phone: json['phone'] as String? ?? '',
    firstName: json['first_name'] as String? ?? '',
    lastName: json['last_name'] as String? ?? '',
    fullName: json['full_name'] as String? ?? '',
    locale: json['locale'] as String? ?? 'uz',
    status: json['status'] as String? ?? 'active',
    phoneVerified: json['phone_verified'] as bool? ?? false,
    roles: _strings(json['roles']),
    permissions: _strings(json['permissions']),
    email: json['email'] as String?,
    avatarUrl: json['avatar_url'] as String?,
    driverId: json['driver_id'] as String?,
    companyId: json['company_id'] as String?,
    companyRole: json['company_role'] as String?,
  );

  Map<String, dynamic> toJson() => <String, dynamic>{
    'id': id,
    'phone': phone,
    'first_name': firstName,
    'last_name': lastName,
    'full_name': fullName,
    'locale': locale,
    'status': status,
    'phone_verified': phoneVerified,
    'roles': roles,
    'permissions': permissions,
    'email': email,
    'avatar_url': avatarUrl,
    'driver_id': driverId,
    'company_id': companyId,
    'company_role': companyRole,
  };

  static List<String> _strings(Object? value) {
    if (value is! List) return const <String>[];
    return value.whereType<String>().toList(growable: false);
  }
}
