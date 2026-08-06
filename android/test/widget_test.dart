import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loadify/app/providers.dart';
import 'package:loadify/core/storage/session_store.dart';
import 'package:loadify/features/auth/presentation/login_screen.dart';
import 'package:shared_preferences/shared_preferences.dart';

Widget _harness(Widget child, SessionStore store) {
  return ProviderScope(
    overrides: <Override>[sessionStoreProvider.overrideWithValue(store)],
    child: MaterialApp(
      locale: const Locale('en'),
      supportedLocales: const <Locale>[Locale('en')],
      localizationsDelegates: const <LocalizationsDelegate<Object>>[
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      home: child,
    ),
  );
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  late SessionStore store;

  setUp(() async {
    SharedPreferences.setMockInitialValues(<String, Object>{
      'loadify.locale': 'en',
    });
    FlutterSecureStorage.setMockInitialValues(<String, String>{});
    store = SessionStore(
      const FlutterSecureStorage(),
      await SharedPreferences.getInstance(),
    );
  });

  testWidgets('rejects a phone that is not +998 followed by nine digits', (
    WidgetTester tester,
  ) async {
    await tester.pumpWidget(_harness(const LoginScreen(), store));
    await tester.pumpAndSettle();

    // The field starts at the country code, which is the point: a driver
    // should not retype +998 on every sign-in.
    expect(find.text('+998'), findsOneWidget);

    await tester.enterText(find.byType(TextFormField).first, '+99890');
    await tester.enterText(find.byType(TextFormField).last, 'Loadify123');
    await tester.tap(find.widgetWithText(FilledButton, 'Sign in'));
    await tester.pump();

    // Caught locally rather than spending a round trip to be told the same.
    expect(find.text('Format: +998XXXXXXXXX'), findsOneWidget);
  });

  testWidgets('rejects a short password', (WidgetTester tester) async {
    await tester.pumpWidget(_harness(const LoginScreen(), store));
    await tester.pumpAndSettle();

    await tester.enterText(find.byType(TextFormField).first, '+998901234567');
    await tester.enterText(find.byType(TextFormField).last, 'short');
    await tester.tap(find.widgetWithText(FilledButton, 'Sign in'));
    await tester.pump();

    expect(find.text('At least 8 characters'), findsOneWidget);
  });

  testWidgets('the password is obscured until revealed', (
    WidgetTester tester,
  ) async {
    await tester.pumpWidget(_harness(const LoginScreen(), store));
    await tester.pumpAndSettle();

    EditableText passwordField() => tester.widget<EditableText>(
      find.descendant(
        of: find.byType(TextFormField).last,
        matching: find.byType(EditableText),
      ),
    );

    expect(passwordField().obscureText, isTrue);

    await tester.tap(find.byIcon(Icons.visibility_outlined));
    await tester.pump();

    expect(passwordField().obscureText, isFalse);
  });
}
