import '../core/format/formatters.dart';

/// Translations.
///
/// Hand-written rather than generated from ARB: the string set is small, and a
/// plain map keeps the app buildable without a codegen step in CI. If this
/// grows past a few hundred keys it should move to flutter_localizations
/// proper.
///
/// Reference data — region names, vehicle type labels, cargo types — is NOT
/// here. The server localises those for the request's Accept-Language, so
/// translating them again on the client would guarantee the two drift.
class S {
  const S(this.locale);

  final String locale;

  static const List<String> supported = <String>['uz', 'ru', 'en'];

  static const Map<String, String> languageNames = <String, String>{
    'uz': "O'zbekcha",
    'ru': 'Русский',
    'en': 'English',
  };

  String _(String key) => _table[locale]?[key] ?? _table['en']?[key] ?? key;

  // Navigation
  String get board => _('board');
  String get bookings => _('bookings');
  String get trips => _('trips');
  String get profile => _('profile');

  // Board
  String get boardTitle => _('boardTitle');
  String get loadsFound => _('loadsFound');
  String get filters => _('filters');
  String get from => _('from');
  String get to => _('to');
  String get anyRegion => _('anyRegion');
  String get vehicleType => _('vehicleType');
  String get anyVehicle => _('anyVehicle');
  String get minWeight => _('minWeight');
  String get maxWeight => _('maxWeight');
  String get fitsMyVehicle => _('fitsMyVehicle');
  String get clearFilters => _('clearFilters');
  String get reserve => _('reserve');
  String get reserved => _('reserved');
  String get contactHidden => _('contactHidden');
  String get perKm => _('perKm');
  String get views => _('views');
  String get emptyBoard => _('emptyBoard');
  String get emptyBoardHint => _('emptyBoardHint');

  // Load detail
  String get cargo => _('cargo');
  String get weight => _('weight');
  String get route => _('route');
  String get pickupDate => _('pickupDate');
  String get shipper => _('shipper');
  String get requirements => _('requirements');
  String get adr => _('adr');
  String get refrigerated => _('refrigerated');
  String get call => _('call');

  // Bookings
  String get bookingsTitle => _('bookingsTitle');
  String get awaitingResponse => _('awaitingResponse');
  String get accept => _('accept');
  String get reject => _('reject');
  String get withdraw => _('withdraw');
  String get noBookings => _('noBookings');

  // Trips
  String get tripsTitle => _('tripsTitle');
  String get activeTrips => _('activeTrips');
  String get pastTrips => _('pastTrips');
  String get nextStage => _('nextStage');
  String get commission => _('commission');
  String get payout => _('payout');
  String get noTrips => _('noTrips');
  String get awaitingConfirmation => _('awaitingConfirmation');
  String get tracking => _('tracking');
  String get trackingOff => _('trackingOff');

  // Trip stages
  String stage(String wire) => _('stage.$wire');

  // Profile
  String get online => _('online');
  String get offline => _('offline');
  String get busy => _('busy');
  String get goOnline => _('goOnline');
  String get vehicles => _('vehicles');
  String get wallet => _('wallet');
  String get balance => _('balance');
  String get held => _('held');
  String get available => _('available');
  String get completedTrips => _('completedTrips');
  String get rating => _('rating');
  String get verified => _('verified');
  String get unverified => _('unverified');
  String get language => _('language');
  String get signOut => _('signOut');

  // Auth
  String get signIn => _('signIn');
  String get signInSubtitle => _('signInSubtitle');
  String get phone => _('phone');
  String get password => _('password');
  String get submit => _('submit');
  String get submitting => _('submitting');
  String get phoneFormat => _('phoneFormat');
  String get passwordTooShort => _('passwordTooShort');
  String get sessionExpired => _('sessionExpired');

  // Generic
  String get retry => _('retry');
  String get loading => _('loading');
  String get error => _('error');
  String get cancel => _('cancel');

  RelativeStrings get relative => switch (locale) {
    'ru' => RelativeStrings(
      now: 'только что',
      minutes: (int n) => '$n мин',
      hours: (int n) => '$n ч',
      days: (int n) => '$n дн',
      inPast: (String a) => '$a назад',
      inFuture: (String a) => 'через $a',
    ),
    'en' => RelativeStrings(
      now: 'just now',
      minutes: (int n) => '$n min',
      hours: (int n) => '${n}h',
      days: (int n) => '${n}d',
      inPast: (String a) => '$a ago',
      inFuture: (String a) => 'in $a',
    ),
    _ => RelativeStrings(
      now: 'hozir',
      minutes: (int n) => '$n daqiqa',
      hours: (int n) => '$n soat',
      days: (int n) => '$n kun',
      inPast: (String a) => '$a oldin',
      inFuture: (String a) => '$a dan keyin',
    ),
  };

  static const Map<String, Map<String, String>> _table =
      <String, Map<String, String>>{
        'uz': <String, String>{
          'board': 'Yuklar',
          'bookings': 'Bronlar',
          'trips': 'Reyslar',
          'profile': 'Profil',
          'boardTitle': 'Yuklar bozori',
          'loadsFound': 'yuk topildi',
          'filters': 'Filtrlar',
          'from': 'Qayerdan',
          'to': 'Qayerga',
          'anyRegion': 'Barcha viloyatlar',
          'vehicleType': 'Mashina turi',
          'anyVehicle': 'Har qanday',
          'minWeight': 'Min ogʻirlik (kg)',
          'maxWeight': 'Maks ogʻirlik (kg)',
          'fitsMyVehicle': 'Faqat mening mashinamga mos',
          'clearFilters': 'Tozalash',
          'reserve': 'Bron qilish',
          'reserved': 'Bron qilingan',
          'contactHidden': 'Bron qilgach koʻrinadi',
          'perKm': '/ km uchun',
          'views': 'koʻrildi',
          'emptyBoard': 'Yuklar topilmadi',
          'emptyBoardHint': 'Filtrlarni kengaytirib koʻring',
          'cargo': 'Yuk',
          'weight': 'Ogʻirlik',
          'route': 'Yoʻnalish',
          'pickupDate': 'Yuklash sanasi',
          'shipper': 'Yuk beruvchi',
          'requirements': 'Talablar',
          'adr': 'ADR',
          'refrigerated': 'Sovutgich',
          'call': 'Qoʻngʻiroq',
          'bookingsTitle': 'Mening bronlarim',
          'awaitingResponse': 'Javob kutilmoqda',
          'accept': 'Qabul qilish',
          'reject': 'Rad etish',
          'withdraw': 'Bekor qilish',
          'noBookings': 'Bronlar yoʻq',
          'tripsTitle': 'Reyslar',
          'activeTrips': 'Faol',
          'pastTrips': 'Tugagan',
          'nextStage': 'Keyingi bosqich',
          'commission': 'Komissiya',
          'payout': 'Haydovchiga',
          'noTrips': 'Reyslar yoʻq',
          'awaitingConfirmation': 'Yuk beruvchi tasdiqlashini kuting',
          'tracking': 'Joylashuv yuborilmoqda',
          'trackingOff': 'Joylashuv oʻchirilgan',
          'stage.created': 'Boshlanmagan',
          'stage.en_route_to_pickup': 'Yuklashga ketmoqda',
          'stage.at_pickup': 'Yuklash joyida',
          'stage.loaded': 'Yuklandi',
          'stage.in_transit': 'Yoʻlda',
          'stage.at_delivery': 'Yetkazish joyida',
          'stage.delivered': 'Yetkazildi',
          'stage.completed': 'Yakunlandi',
          'stage.cancelled': 'Bekor qilindi',
          'stage.disputed': 'Nizo',
          'online': 'Onlayn',
          'offline': 'Oflayn',
          'busy': 'Band',
          'goOnline': 'Ish qabul qilish',
          'vehicles': 'Mashinalar',
          'wallet': 'Hamyon',
          'balance': 'Balans',
          'held': 'Bloklangan',
          'available': 'Mavjud',
          'completedTrips': 'Yakunlangan reyslar',
          'rating': 'Reyting',
          'verified': 'Tasdiqlangan',
          'unverified': 'Tasdiqlanmagan',
          'language': 'Til',
          'signOut': 'Chiqish',
          'signIn': 'Kirish',
          'signInSubtitle': 'Telefon raqamingiz bilan kiring',
          'phone': 'Telefon',
          'password': 'Parol',
          'submit': 'Kirish',
          'submitting': 'Kirilmoqda...',
          'phoneFormat': 'Format: +998XXXXXXXXX',
          'passwordTooShort': 'Kamida 8 ta belgi',
          'sessionExpired': 'Sessiya tugadi, qayta kiring',
          'retry': 'Qayta urinish',
          'loading': 'Yuklanmoqda...',
          'error': 'Xatolik yuz berdi',
          'cancel': 'Bekor qilish',
        },
        'ru': <String, String>{
          'board': 'Грузы',
          'bookings': 'Брони',
          'trips': 'Рейсы',
          'profile': 'Профиль',
          'boardTitle': 'Биржа грузов',
          'loadsFound': 'грузов найдено',
          'filters': 'Фильтры',
          'from': 'Откуда',
          'to': 'Куда',
          'anyRegion': 'Все регионы',
          'vehicleType': 'Тип кузова',
          'anyVehicle': 'Любой',
          'minWeight': 'Мин. вес (кг)',
          'maxWeight': 'Макс. вес (кг)',
          'fitsMyVehicle': 'Только под мою машину',
          'clearFilters': 'Сбросить',
          'reserve': 'Забронировать',
          'reserved': 'Забронировано',
          'contactHidden': 'Откроется после брони',
          'perKm': '/ за км',
          'views': 'просмотров',
          'emptyBoard': 'Грузы не найдены',
          'emptyBoardHint': 'Попробуйте расширить фильтры',
          'cargo': 'Груз',
          'weight': 'Вес',
          'route': 'Маршрут',
          'pickupDate': 'Дата загрузки',
          'shipper': 'Грузоотправитель',
          'requirements': 'Требования',
          'adr': 'ADR',
          'refrigerated': 'Рефрижератор',
          'call': 'Позвонить',
          'bookingsTitle': 'Мои брони',
          'awaitingResponse': 'Ожидает ответа',
          'accept': 'Принять',
          'reject': 'Отклонить',
          'withdraw': 'Отозвать',
          'noBookings': 'Броней нет',
          'tripsTitle': 'Рейсы',
          'activeTrips': 'Активные',
          'pastTrips': 'Завершённые',
          'nextStage': 'Следующий этап',
          'commission': 'Комиссия',
          'payout': 'Водителю',
          'noTrips': 'Рейсов нет',
          'awaitingConfirmation': 'Ожидает подтверждения отправителя',
          'tracking': 'Геопозиция передаётся',
          'trackingOff': 'Геопозиция выключена',
          'stage.created': 'Не начат',
          'stage.en_route_to_pickup': 'Едет на загрузку',
          'stage.at_pickup': 'На загрузке',
          'stage.loaded': 'Загружен',
          'stage.in_transit': 'В пути',
          'stage.at_delivery': 'На выгрузке',
          'stage.delivered': 'Доставлен',
          'stage.completed': 'Завершён',
          'stage.cancelled': 'Отменён',
          'stage.disputed': 'Спор',
          'online': 'Онлайн',
          'offline': 'Офлайн',
          'busy': 'Занят',
          'goOnline': 'Принимать заказы',
          'vehicles': 'Машины',
          'wallet': 'Кошелёк',
          'balance': 'Баланс',
          'held': 'Заблокировано',
          'available': 'Доступно',
          'completedTrips': 'Завершённых рейсов',
          'rating': 'Рейтинг',
          'verified': 'Проверен',
          'unverified': 'Не проверен',
          'language': 'Язык',
          'signOut': 'Выйти',
          'signIn': 'Вход',
          'signInSubtitle': 'Войдите по номеру телефона',
          'phone': 'Телефон',
          'password': 'Пароль',
          'submit': 'Войти',
          'submitting': 'Вход...',
          'phoneFormat': 'Формат: +998XXXXXXXXX',
          'passwordTooShort': 'Минимум 8 символов',
          'sessionExpired': 'Сессия истекла, войдите снова',
          'retry': 'Повторить',
          'loading': 'Загрузка...',
          'error': 'Произошла ошибка',
          'cancel': 'Отмена',
        },
        'en': <String, String>{
          'board': 'Loads',
          'bookings': 'Bookings',
          'trips': 'Trips',
          'profile': 'Profile',
          'boardTitle': 'Load board',
          'loadsFound': 'loads found',
          'filters': 'Filters',
          'from': 'From',
          'to': 'To',
          'anyRegion': 'All regions',
          'vehicleType': 'Vehicle type',
          'anyVehicle': 'Any',
          'minWeight': 'Min weight (kg)',
          'maxWeight': 'Max weight (kg)',
          'fitsMyVehicle': 'Only loads my vehicle fits',
          'clearFilters': 'Clear',
          'reserve': 'Reserve',
          'reserved': 'Reserved',
          'contactHidden': 'Visible after booking',
          'perKm': '/ per km',
          'views': 'views',
          'emptyBoard': 'No loads found',
          'emptyBoardHint': 'Try widening your filters',
          'cargo': 'Cargo',
          'weight': 'Weight',
          'route': 'Route',
          'pickupDate': 'Pickup date',
          'shipper': 'Shipper',
          'requirements': 'Requirements',
          'adr': 'ADR',
          'refrigerated': 'Refrigerated',
          'call': 'Call',
          'bookingsTitle': 'My bookings',
          'awaitingResponse': 'Awaiting response',
          'accept': 'Accept',
          'reject': 'Reject',
          'withdraw': 'Withdraw',
          'noBookings': 'No bookings yet',
          'tripsTitle': 'Trips',
          'activeTrips': 'Active',
          'pastTrips': 'Past',
          'nextStage': 'Next stage',
          'commission': 'Commission',
          'payout': 'Payout',
          'noTrips': 'No trips yet',
          'awaitingConfirmation': 'Waiting for the shipper to confirm',
          'tracking': 'Sharing location',
          'trackingOff': 'Location off',
          'stage.created': 'Not started',
          'stage.en_route_to_pickup': 'En route to pickup',
          'stage.at_pickup': 'At pickup',
          'stage.loaded': 'Loaded',
          'stage.in_transit': 'In transit',
          'stage.at_delivery': 'At delivery',
          'stage.delivered': 'Delivered',
          'stage.completed': 'Completed',
          'stage.cancelled': 'Cancelled',
          'stage.disputed': 'Disputed',
          'online': 'Online',
          'offline': 'Offline',
          'busy': 'Busy',
          'goOnline': 'Accepting work',
          'vehicles': 'Vehicles',
          'wallet': 'Wallet',
          'balance': 'Balance',
          'held': 'Held',
          'available': 'Available',
          'completedTrips': 'Completed trips',
          'rating': 'Rating',
          'verified': 'Verified',
          'unverified': 'Unverified',
          'language': 'Language',
          'signOut': 'Sign out',
          'signIn': 'Sign in',
          'signInSubtitle': 'Sign in with your phone number',
          'phone': 'Phone',
          'password': 'Password',
          'submit': 'Sign in',
          'submitting': 'Signing in...',
          'phoneFormat': 'Format: +998XXXXXXXXX',
          'passwordTooShort': 'At least 8 characters',
          'sessionExpired': 'Session expired, sign in again',
          'retry': 'Retry',
          'loading': 'Loading...',
          'error': 'Something went wrong',
          'cancel': 'Cancel',
        },
      };
}
