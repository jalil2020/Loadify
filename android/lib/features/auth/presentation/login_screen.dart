import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../app/locale.dart';
import '../../../app/providers.dart';
import '../../../core/error/api_exception.dart';
import '../../../l10n/strings.dart';

class LoginScreen extends ConsumerStatefulWidget {
  const LoginScreen({super.key});

  @override
  ConsumerState<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends ConsumerState<LoginScreen> {
  final GlobalKey<FormState> _form = GlobalKey<FormState>();
  final TextEditingController _phone = TextEditingController(text: '+998');
  final TextEditingController _password = TextEditingController();

  bool _submitting = false;
  bool _obscure = true;
  String? _formError;

  @override
  void dispose() {
    _phone.dispose();
    _password.dispose();
    super.dispose();
  }

  Future<void> _submit(S s) async {
    if (!(_form.currentState?.validate() ?? false)) return;

    setState(() {
      _submitting = true;
      _formError = null;
    });

    try {
      await ref
          .read(authProvider.notifier)
          .signIn(_phone.text.trim(), _password.text);
      // The router redirects on the session change; no navigation here, so a
      // sign-in from a deep link returns to where the driver was headed.
    } on ApiException catch (e) {
      if (!mounted) return;
      // Field errors belong on the field. Only what cannot be attributed to an
      // input becomes a form-level message.
      setState(() => _formError = e.fieldErrors.isEmpty ? e.message : null);
      if (e.fieldErrors.isNotEmpty) _form.currentState?.validate();
    } on Object {
      if (!mounted) return;
      setState(() => _formError = s.error);
    } finally {
      if (mounted) setState(() => _submitting = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final S s = ref.watch(stringsProvider);
    final ColorScheme scheme = Theme.of(context).colorScheme;

    return Scaffold(
      body: SafeArea(
        child: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(24),
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 420),
              child: Form(
                key: _form,
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: <Widget>[
                    Icon(
                      Icons.local_shipping_rounded,
                      size: 52,
                      color: scheme.primary,
                    ),
                    const SizedBox(height: 12),
                    Text(
                      'Loadify',
                      textAlign: TextAlign.center,
                      style: Theme.of(context).textTheme.headlineSmall
                          ?.copyWith(fontWeight: FontWeight.w700),
                    ),
                    const SizedBox(height: 4),
                    Text(
                      s.signInSubtitle,
                      textAlign: TextAlign.center,
                      style: Theme.of(context).textTheme.bodySmall?.copyWith(
                        color: scheme.onSurfaceVariant,
                      ),
                    ),
                    const SizedBox(height: 28),

                    TextFormField(
                      controller: _phone,
                      keyboardType: TextInputType.phone,
                      autofillHints: const <String>[
                        AutofillHints.telephoneNumber,
                      ],
                      decoration: InputDecoration(
                        labelText: s.phone,
                        hintText: '+998901234567',
                        prefixIcon: const Icon(Icons.phone_outlined),
                      ),
                      validator: (String? v) =>
                          RegExp(r'^\+998\d{9}$').hasMatch(v?.trim() ?? '')
                          ? null
                          : s.phoneFormat,
                    ),
                    const SizedBox(height: 14),

                    TextFormField(
                      controller: _password,
                      obscureText: _obscure,
                      autofillHints: const <String>[AutofillHints.password],
                      decoration: InputDecoration(
                        labelText: s.password,
                        prefixIcon: const Icon(Icons.lock_outline_rounded),
                        suffixIcon: IconButton(
                          onPressed: () => setState(() => _obscure = !_obscure),
                          icon: Icon(
                            _obscure
                                ? Icons.visibility_outlined
                                : Icons.visibility_off_outlined,
                          ),
                        ),
                      ),
                      onFieldSubmitted: (_) => _submit(s),
                      validator: (String? v) =>
                          (v ?? '').length >= 8 ? null : s.passwordTooShort,
                    ),

                    if (_formError != null) ...<Widget>[
                      const SizedBox(height: 14),
                      Container(
                        padding: const EdgeInsets.all(12),
                        decoration: BoxDecoration(
                          color: scheme.errorContainer,
                          borderRadius: BorderRadius.circular(12),
                        ),
                        child: Text(
                          _formError!,
                          style: TextStyle(color: scheme.onErrorContainer),
                        ),
                      ),
                    ],

                    const SizedBox(height: 22),
                    FilledButton(
                      onPressed: _submitting ? null : () => _submit(s),
                      child: _submitting
                          ? const SizedBox(
                              width: 20,
                              height: 20,
                              child: CircularProgressIndicator(strokeWidth: 2),
                            )
                          : Text(s.submit),
                    ),

                    const SizedBox(height: 20),
                    Wrap(
                      alignment: WrapAlignment.center,
                      spacing: 8,
                      children: S.supported
                          .map(
                            (String code) => ChoiceChip(
                              label: Text(S.languageNames[code] ?? code),
                              selected: ref.watch(localeProvider) == code,
                              onSelected: (_) =>
                                  ref.read(localeProvider.notifier).set(code),
                            ),
                          )
                          .toList(),
                    ),
                  ],
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}
