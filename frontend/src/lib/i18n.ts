import type { Locale } from "@/lib/types";

/**
 * Translations live in one typed object rather than JSON files.
 *
 * The dictionary is small enough that the type safety — a missing key is a
 * compile error, not a blank label in production — is worth more than the
 * tooling a full i18n library brings.
 */
const dictionary = {
  uz: {
    "app.name": "Loadify",
    "app.tagline": "Yuk va haydovchilarni real vaqtda bogʻlaydi",

    "nav.board": "Yuklar",
    "nav.myLoads": "Mening yuklarim",
    "nav.bookings": "Bronlar",
    "nav.trips": "Reyslar",
    "nav.drivers": "Haydovchilar",
    "nav.wallet": "Hamyon",
    "nav.admin": "Boshqaruv",
    "nav.signIn": "Kirish",
    "nav.signOut": "Chiqish",

    "auth.signIn": "Tizimga kirish",
    "auth.signInSubtitle": "Telefon raqamingiz va parolingizni kiriting",
    "auth.phone": "Telefon raqam",
    "auth.password": "Parol",
    "auth.submit": "Kirish",
    "auth.submitting": "Kirilmoqda…",
    "auth.demoHint": "Demo hisoblar",

    "board.title": "Yuklar bozori",
    "board.subtitle": "yuk topildi",
    "board.empty": "Filtrlarga mos yuk topilmadi",
    "board.emptyHint": "Filtrlarni kengaytirib koʻring",
    "board.fitsMyVehicle": "Faqat mening mashinamga mos",
    "board.reserve": "Bron qilish",
    "board.reserved": "Bron qilingan",
    "board.reserving": "Bron qilinmoqda…",
    "board.contactHidden": "Bron qilgach koʻrinadi",

    "search.label": "Aqlli qidiruv",
    "search.placeholder": "Toshkentdan Samarqandga ertaga 20 tonna tent",
    "search.submit": "Qidirish",
    "search.removeChip": "olib tashlash",
    "search.dropped": "Tushunilmadi:",
    "search.notUnderstood": "Soʻrov tushunilmadi. Viloyat nomi va yuk turini yozib koʻring.",

    "filter.title": "Filtrlar",
    "filter.from": "Qayerdan",
    "filter.to": "Qayerga",
    "filter.anyRegion": "Barcha viloyatlar",
    "filter.vehicleType": "Mashina turi",
    "filter.anyVehicle": "Har qanday",
    "filter.minWeight": "Min ogʻirlik (kg)",
    "filter.maxWeight": "Maks ogʻirlik (kg)",
    "filter.reset": "Tozalash",

    "load.weight": "Ogʻirlik",
    "load.distance": "Masofa",
    "load.price": "Narx",
    "load.pickup": "Yuklash",
    "load.delivery": "Tushirish",
    "load.vehicle": "Mashina",
    "load.perKm": "km uchun",
    "load.urgent": "Shoshilinch",
    "load.adr": "ADR",
    "load.refrigerated": "Refrijerator",
    "load.views": "koʻrildi",

    "loads.new": "Yangi yuk",
    "loads.mine": "Mening yuklarim",
    "loads.publish": "Eʼlon qilish",
    "loads.cancel": "Bekor qilish",

    "booking.accept": "Qabul qilish",
    "booking.reject": "Rad etish",
    "booking.pending": "Javob kutilmoqda",
    "booking.empty": "Hozircha bronlar yoʻq",

    "trip.advance": "Keyingi bosqich",
    "trip.confirm": "Yetkazilganini tasdiqlash",
    "trip.payout": "Haydovchiga",
    "trip.commission": "Komissiya",
    "trip.empty": "Hozircha reyslar yoʻq",

    "wallet.balance": "Balans",
    "wallet.available": "Mavjud",
    "wallet.held": "Bloklangan",

    "admin.dashboard": "Boshqaruv paneli",
    "admin.corridors": "Yoʻnalishlar",
    "admin.gmv": "Aylanma",
    "admin.commission": "Komissiya",

    "common.loading": "Yuklanmoqda…",
    "common.error": "Xatolik yuz berdi",
    "common.retry": "Qayta urinish",
    "common.all": "Barchasi",
    "common.status": "Holat",
    "common.actions": "Amallar",
    "common.of": "dan",
    "common.page": "sahifa",
    "common.prev": "Oldingi",
    "common.next": "Keyingi",
  },

  ru: {
    "app.name": "Loadify",
    "app.tagline": "Связывает грузы и водителей в реальном времени",

    "nav.board": "Грузы",
    "nav.myLoads": "Мои грузы",
    "nav.bookings": "Брони",
    "nav.trips": "Рейсы",
    "nav.drivers": "Водители",
    "nav.wallet": "Кошелёк",
    "nav.admin": "Админка",
    "nav.signIn": "Войти",
    "nav.signOut": "Выйти",

    "auth.signIn": "Вход в систему",
    "auth.signInSubtitle": "Введите номер телефона и пароль",
    "auth.phone": "Номер телефона",
    "auth.password": "Пароль",
    "auth.submit": "Войти",
    "auth.submitting": "Вход…",
    "auth.demoHint": "Демо-аккаунты",

    "board.title": "Биржа грузов",
    "board.subtitle": "грузов найдено",
    "board.empty": "По фильтрам ничего не найдено",
    "board.emptyHint": "Попробуйте расширить фильтры",
    "board.fitsMyVehicle": "Только под мою машину",
    "board.reserve": "Забронировать",
    "board.reserved": "Забронировано",
    "board.reserving": "Бронируем…",
    "board.contactHidden": "Откроется после брони",

    "search.label": "Умный поиск",
    "search.placeholder": "из Бухары в Самарканд завтра 20 тонн тент",
    "search.submit": "Найти",
    "search.removeChip": "убрать",
    "search.dropped": "Не распознано:",
    "search.notUnderstood": "Запрос не распознан. Укажите регион и тип груза.",

    "filter.title": "Фильтры",
    "filter.from": "Откуда",
    "filter.to": "Куда",
    "filter.anyRegion": "Все регионы",
    "filter.vehicleType": "Тип машины",
    "filter.anyVehicle": "Любой",
    "filter.minWeight": "Мин. вес (кг)",
    "filter.maxWeight": "Макс. вес (кг)",
    "filter.reset": "Сбросить",

    "load.weight": "Вес",
    "load.distance": "Расстояние",
    "load.price": "Цена",
    "load.pickup": "Погрузка",
    "load.delivery": "Выгрузка",
    "load.vehicle": "Машина",
    "load.perKm": "за км",
    "load.urgent": "Срочно",
    "load.adr": "ADR",
    "load.refrigerated": "Рефрижератор",
    "load.views": "просмотров",

    "loads.new": "Новый груз",
    "loads.mine": "Мои грузы",
    "loads.publish": "Опубликовать",
    "loads.cancel": "Отменить",

    "booking.accept": "Принять",
    "booking.reject": "Отклонить",
    "booking.pending": "Ожидает ответа",
    "booking.empty": "Броней пока нет",

    "trip.advance": "Следующий этап",
    "trip.confirm": "Подтвердить доставку",
    "trip.payout": "Водителю",
    "trip.commission": "Комиссия",
    "trip.empty": "Рейсов пока нет",

    "wallet.balance": "Баланс",
    "wallet.available": "Доступно",
    "wallet.held": "Заблокировано",

    "admin.dashboard": "Панель управления",
    "admin.corridors": "Направления",
    "admin.gmv": "Оборот",
    "admin.commission": "Комиссия",

    "common.loading": "Загрузка…",
    "common.error": "Произошла ошибка",
    "common.retry": "Повторить",
    "common.all": "Все",
    "common.status": "Статус",
    "common.actions": "Действия",
    "common.of": "из",
    "common.page": "стр.",
    "common.prev": "Назад",
    "common.next": "Вперёд",
  },

  en: {
    "app.name": "Loadify",
    "app.tagline": "Connects freight and drivers in real time",

    "nav.board": "Loads",
    "nav.myLoads": "My loads",
    "nav.bookings": "Bookings",
    "nav.trips": "Trips",
    "nav.drivers": "Drivers",
    "nav.wallet": "Wallet",
    "nav.admin": "Admin",
    "nav.signIn": "Sign in",
    "nav.signOut": "Sign out",

    "auth.signIn": "Sign in",
    "auth.signInSubtitle": "Enter your phone number and password",
    "auth.phone": "Phone number",
    "auth.password": "Password",
    "auth.submit": "Sign in",
    "auth.submitting": "Signing in…",
    "auth.demoHint": "Demo accounts",

    "board.title": "Load board",
    "board.subtitle": "loads found",
    "board.empty": "No loads match these filters",
    "board.emptyHint": "Try widening the filters",
    "board.fitsMyVehicle": "Only what my truck fits",
    "board.reserve": "Reserve",
    "board.reserved": "Reserved",
    "board.reserving": "Reserving…",
    "board.contactHidden": "Shown after you reserve",

    "search.label": "Smart search",
    "search.placeholder": "from Bukhara to Samarkand tomorrow 20 t tent",
    "search.submit": "Search",
    "search.removeChip": "remove",
    "search.dropped": "Not understood:",
    "search.notUnderstood": "Could not read that. Try naming a region and a body type.",

    "filter.title": "Filters",
    "filter.from": "From",
    "filter.to": "To",
    "filter.anyRegion": "All regions",
    "filter.vehicleType": "Vehicle type",
    "filter.anyVehicle": "Any",
    "filter.minWeight": "Min weight (kg)",
    "filter.maxWeight": "Max weight (kg)",
    "filter.reset": "Reset",

    "load.weight": "Weight",
    "load.distance": "Distance",
    "load.price": "Price",
    "load.pickup": "Pickup",
    "load.delivery": "Delivery",
    "load.vehicle": "Vehicle",
    "load.perKm": "per km",
    "load.urgent": "Urgent",
    "load.adr": "ADR",
    "load.refrigerated": "Refrigerated",
    "load.views": "views",

    "loads.new": "New load",
    "loads.mine": "My loads",
    "loads.publish": "Publish",
    "loads.cancel": "Cancel",

    "booking.accept": "Accept",
    "booking.reject": "Reject",
    "booking.pending": "Awaiting response",
    "booking.empty": "No bookings yet",

    "trip.advance": "Next stage",
    "trip.confirm": "Confirm delivery",
    "trip.payout": "Driver payout",
    "trip.commission": "Commission",
    "trip.empty": "No trips yet",

    "wallet.balance": "Balance",
    "wallet.available": "Available",
    "wallet.held": "Held",

    "admin.dashboard": "Dashboard",
    "admin.corridors": "Corridors",
    "admin.gmv": "GMV",
    "admin.commission": "Commission",

    "common.loading": "Loading…",
    "common.error": "Something went wrong",
    "common.retry": "Try again",
    "common.all": "All",
    "common.status": "Status",
    "common.actions": "Actions",
    "common.of": "of",
    "common.page": "page",
    "common.prev": "Previous",
    "common.next": "Next",
  },
} as const;

/** Every key must exist in every locale, enforced by this type. */
export type TranslationKey = keyof (typeof dictionary)["en"];

type Dictionary = Record<TranslationKey, string>;
const dictionaries: Record<Locale, Dictionary> = dictionary;

export function translate(locale: Locale, key: TranslationKey): string {
  return dictionaries[locale][key] ?? dictionaries.en[key] ?? key;
}

export const LOCALES: { code: Locale; label: string }[] = [
  { code: "uz", label: "O'zbekcha" },
  { code: "ru", label: "Русский" },
  { code: "en", label: "English" },
];
