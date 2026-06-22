package seed

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	bulkChannelLimit     = 30
	videosPerBulkChannel = 30
)

// seedBulkChannels добавляет 30 каналов и по 30 видео в каждом, итого 900 видео.
// У каждого канала свой владелец-пользователь. Пароль для всех — "password".
// video_url'ы берутся из YouTube-ссылок. Это только системный seed: пользовательская
// загрузка через API принимает только локально загруженные файлы.
func seedBulkChannels(ctx context.Context, tx pgx.Tx, now time.Time) error {
	hash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash bulk password: %w", err)
	}
	h := string(hash)
	rng := rand.New(rand.NewSource(1337))

	limit := bulkChannelLimit
	if len(bulkChannels) < limit {
		limit = len(bulkChannels)
	}

	for i := 0; i < limit; i++ {
		ch := bulkChannels[i]
		userID := fmtUUID("b0", i+1)
		channelID := fmtUUID("c0", i+1)
		username := ch.username

		if _, err := tx.Exec(ctx, `
			INSERT INTO users(id, username, display_name, email, avatar_url, bio, role, password_hash)
			VALUES ($1, $2, $3, $4, '', $5, 'user', $6)
			ON CONFLICT (id) DO NOTHING`,
			userID, username, ch.displayName,
			username+"@videohub.local", ch.bio, &h); err != nil {
			return fmt.Errorf("user %s: %w", username, err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO channels(id, owner_id, name, handle, description, avatar_url, banner_url, subscribers_count)
			VALUES ($1, $2, $3, $4, $5, '', '', $6)
			ON CONFLICT (id) DO NOTHING`,
			channelID, userID, ch.displayName, "@"+username, ch.description,
			int64(1_000+rng.Intn(900_000))); err != nil {
			return fmt.Errorf("channel %s: %w", username, err)
		}

		titles := titlesByCategory[ch.category]
		if len(titles) == 0 {
			titles = titlesByCategory["other"]
		}

		for j := 0; j < videosPerBulkChannel; j++ {
			// Используем единый последовательный номер 1..900 — UUID
			// собирается из hex-цифр, никаких 'v' и других нестандартных символов.
			videoSeq := i*videosPerBulkChannel + j + 1
			videoID := fmtUUID("dd", videoSeq)
			youtubeID := publicYouTubeIDs[(i*videosPerBulkChannel+j)%len(publicYouTubeIDs)]
			videoURL := "https://www.youtube.com/watch?v=" + youtubeID
			thumb := "https://img.youtube.com/vi/" + youtubeID + "/hqdefault.jpg"
			title := titles[j%len(titles)]
			if j >= len(titles) {
				title = fmt.Sprintf("%s · выпуск %d", title, j/len(titles)+1)
			}
			uploadedAt := now.AddDate(0, 0, -rng.Intn(120))
			views := int64(1_000 + rng.Intn(500_000))
			likes := int64(1 + rng.Intn(int(views/10)+1))
			dislikes := int64(1 + rng.Intn(int(likes/5)+1))
			duration := 60 + rng.Intn(1800)
			tags := pickTags(ch.category, rng)

			if _, err := tx.Exec(ctx, `
				INSERT INTO videos
				(id, channel_id, title, description, thumbnail_url, video_url, duration_sec,
				 views_count, likes_count, dislikes_count, category, visibility, tags, uploaded_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'public', $12, $13)
				ON CONFLICT (id) DO UPDATE SET
					title = EXCLUDED.title,
					description = EXCLUDED.description,
					thumbnail_url = EXCLUDED.thumbnail_url,
					video_url = EXCLUDED.video_url,
					duration_sec = EXCLUDED.duration_sec,
					views_count = EXCLUDED.views_count,
					likes_count = EXCLUDED.likes_count,
					dislikes_count = EXCLUDED.dislikes_count,
					category = EXCLUDED.category,
					visibility = EXCLUDED.visibility,
					tags = EXCLUDED.tags,
					uploaded_at = EXCLUDED.uploaded_at`,
				videoID, channelID, title, ch.description, thumb, videoURL, duration,
				views, likes, dislikes, ch.category, tags, uploadedAt); err != nil {
				return fmt.Errorf("video %s/%d: %w", username, j, err)
			}
		}
	}
	return nil
}

// fmtUUID формирует детерминированный валидный UUID-v4-подобный идентификатор
// из коротких префикса+номера, удобно для seed-данных (не пересекается с
// gen_random_uuid и легко узнаётся в БД).
func fmtUUID(prefix string, n int) string {
	hex := fmt.Sprintf("%s%010d", strings.ToLower(prefix), n)
	if len(hex) > 32 {
		hex = hex[:32]
	}
	for len(hex) < 32 {
		hex += "0"
	}
	return fmt.Sprintf("%s-%s-%s-%s-%s", hex[0:8], hex[8:12], hex[12:16], hex[16:20], hex[20:32])
}

// YouTube ID для системных demo seed-видео. Пользовательская загрузка по YouTube-ссылкам запрещена в handlers/videos.go.
var publicYouTubeIDs = []string{
	"dQw4w9WgXcQ",
	"9bZkp7q19f0",
	"kJQP7kiw5Fk",
	"3JZ_D3ELwOQ",
	"RgKAFK5djSk",
	"OPf0YbXqDm0",
	"fJ9rUzIMcZQ",
	"hTWKbfoikeg",
	"JGwWNGJdvx8",
	"60ItHLz5WEA",
	"YQHsXMglC9A",
	"CevxZvSJLk8",
	"L_jWHffIx5E",
}

type bulkChannel struct {
	username    string
	displayName string
	bio         string
	description string
	category    string
}

var bulkChannels = []bulkChannel{
	{"music_planet", "Music Planet", "Музыка со всего мира.", "Клипы, разборы, концерты.", "music"},
	{"beat_drop", "Beat Drop", "Электронная музыка.", "EDM, хаус, техно.", "music"},
	{"rock_arena", "Rock Arena", "Рок-сцена.", "Концерты и альбомы рок-групп.", "music"},
	{"jazz_corner", "Jazz Corner", "Джазовые вечера.", "Стандарты и импровизации.", "music"},
	{"classical_hour", "Classical Hour", "Классическая музыка.", "Симфонии и концерты.", "music"},

	{"tech_world", "Tech World", "Обзоры технологий.", "Гаджеты, смартфоны, ноутбуки.", "tech"},
	{"code_studio", "Code Studio", "Разработка ПО.", "Уроки и разборы кода.", "tech"},
	{"ai_lab", "AI Lab", "Машинное обучение.", "Нейросети простыми словами.", "tech"},
	{"cyber_news", "CyberNews", "Кибербезопасность.", "Новости и кейсы по безопасности.", "tech"},
	{"hardware_zone", "Hardware Zone", "Железо.", "Сборка ПК и обзоры комплектующих.", "tech"},

	{"gamer_pro_2", "GamerPro 2", "Игры и стримы.", "Прохождения и обзоры.", "gaming"},
	{"indie_arcade", "Indie Arcade", "Инди-игры.", "Скрытые жемчужины игровой индустрии.", "gaming"},
	{"retro_quest", "RetroQuest", "Ретро-игры.", "Классика 80–90-х.", "gaming"},
	{"esports_live", "Esports Live", "Киберспорт.", "Матчи и аналитика.", "gaming"},
	{"speedrun_zone", "Speedrun Zone", "Спидраны.", "Игры на скорость прохождения.", "gaming"},

	{"edu_class", "Edu Class", "Образование.", "Математика, физика, история.", "education"},
	{"math_master", "Math Master", "Математика.", "От алгебры до анализа.", "education"},
	{"history_hub", "History Hub", "История.", "Великие события и личности.", "education"},
	{"language_lab", "Language Lab", "Языки.", "Английский, немецкий, испанский.", "education"},
	{"science_pop", "SciencePop", "Популярная наука.", "Физика, химия, биология.", "education"},

	{"sport_zone_2", "Sport Zone 2", "Спорт.", "Тренировки и спортивные обзоры.", "sports"},
	{"fitness_daily", "Fitness Daily", "Фитнес.", "Ежедневные тренировки.", "sports"},
	{"yoga_flow", "Yoga Flow", "Йога и осознанность.", "Йога-классы и медитация.", "sports"},
	{"runner_path", "Runner Path", "Бег и марафон.", "Тренировки бегунов.", "sports"},
	{"team_play", "Team Play", "Командные виды спорта.", "Футбол, баскетбол, хоккей.", "sports"},

	{"news_today", "News Today", "Новости.", "Главные события дня.", "news"},
	{"world_brief", "World Brief", "Мировые новости.", "Аналитика международных событий.", "news"},

	{"comedy_club", "Comedy Club", "Юмор.", "Стендап и скетчи.", "entertainment"},
	{"movie_buff", "Movie Buff", "Кино.", "Обзоры фильмов и сериалов.", "entertainment"},
	{"book_shelf", "Book Shelf", "Литература.", "Рецензии книг.", "entertainment"},
	{"talk_show", "Talk Show", "Ток-шоу.", "Интервью с интересными людьми.", "entertainment"},
	{"trivia_time", "Trivia Time", "Викторины.", "Интеллектуальные игры.", "entertainment"},

	{"cooking_master_2", "Cooking Master 2", "Кулинария.", "Рецепты и кухонные лайфхаки.", "other"},
	{"baking_lab", "Baking Lab", "Выпечка.", "Хлеб, торты, десерты.", "other"},
	{"street_food", "Street Food", "Стрит-фуд.", "Еда со всего мира.", "other"},

	{"travel_world_2", "Travel World 2", "Путешествия.", "Маршруты и впечатления.", "other"},
	{"city_walks", "City Walks", "Городские прогулки.", "Виды и истории городов.", "other"},
	{"nature_view", "Nature View", "Природа.", "Леса, горы, океаны.", "other"},
	{"camp_life", "Camp Life", "Кемпинг.", "Палатки, костёр, природа.", "other"},

	{"art_studio_2", "Art Studio 2", "Искусство.", "Уроки рисования и обзоры выставок.", "other"},
	{"design_lab", "Design Lab", "Графический дизайн.", "Уроки Photoshop и Figma.", "other"},
	{"photo_master", "Photo Master", "Фотография.", "Композиция и обработка снимков.", "other"},

	{"diy_workshop", "DIY Workshop", "Сделай сам.", "Поделки и ремонт своими руками.", "other"},
	{"home_garden", "Home & Garden", "Дом и сад.", "Растения и уют в доме.", "other"},
	{"pet_friends", "Pet Friends", "Питомцы.", "Уход и обучение животных.", "other"},

	{"finance_now", "Finance Now", "Финансы.", "Инвестиции и личные финансы.", "other"},
	{"crypto_chain", "Crypto Chain", "Криптовалюты.", "Анализ рынка криптовалют.", "other"},
	{"business_brief", "Business Brief", "Бизнес.", "Запуск и развитие бизнеса.", "other"},
	{"life_hacks", "Life Hacks", "Лайфхаки.", "Полезные советы на каждый день.", "other"},
	{"car_garage", "Car Garage", "Автомобили.", "Обзоры авто и ремонт.", "other"},
}

func pickTags(category string, rng *rand.Rand) []string {
	pool := tagsByCategory[category]
	if len(pool) == 0 {
		pool = tagsByCategory["other"]
	}
	n := 2 + rng.Intn(2)
	chosen := make([]string, 0, n)
	for i := 0; i < n; i++ {
		chosen = append(chosen, pool[rng.Intn(len(pool))])
	}
	return chosen
}

var tagsByCategory = map[string][]string{
	"music":         {"music", "клипы", "live", "альбом", "разбор"},
	"tech":          {"tech", "обзор", "смартфон", "ноутбук", "гаджет"},
	"gaming":        {"games", "прохождение", "обзор", "стрим", "ретро"},
	"education":     {"наука", "уроки", "математика", "физика", "лекция"},
	"sports":        {"sport", "фитнес", "тренировка", "марафон", "разминка"},
	"news":          {"news", "аналитика", "сегодня"},
	"entertainment": {"юмор", "интервью", "обзор", "разбор"},
	"other":         {"vlog", "обзор", "история", "разбор", "топ"},
}

var titlesByCategory = map[string][]string{
	"music": {
		"Концерт под открытым небом",
		"Разбор главного хита недели",
		"История одного альбома",
		"Топ-10 синглов лета",
		"Студийная сессия легендарной группы",
		"Музыкальные инструменты с нуля",
		"Аранжировка простой мелодии",
		"Гид по жанрам современной музыки",
		"Лучшие саундтреки 2020-х",
		"Эксклюзивное выступление в студии",
		"Как звучит акустика старого зала",
		"Разбор битов хип-хоп классики",
		"Что слушать на пробежке",
		"Закулисье большого тура",
		"Дуэт двух исполнителей",
	},
	"tech": {
		"Распаковка флагмана 2026",
		"Что нового в обновлении ОС",
		"5 приложений, которые меняют день",
		"Гид по выбору ноутбука",
		"Сравнение топовых наушников",
		"Тест камеры в темноте",
		"Как ускорить старый ПК",
		"Обзор беспроводной зарядки",
		"Разбор архитектуры процессора",
		"Идеи для умного дома",
		"Сборка ПК за час",
		"Тест автономности на 24 часа",
		"Лучшие лаптопы для разработчика",
		"Гаджеты, которые удивили в этом году",
		"Что выбрать: SSD или HDD",
	},
	"gaming": {
		"Прохождение на максимальной сложности",
		"Топ-5 инди-игр месяца",
		"Скрытые пасхалки знаменитой серии",
		"Разбор боевой системы",
		"Гид новичка по жанру",
		"Реакция стримера на твист сюжета",
		"Сравнение версий на разных платформах",
		"Спидран за 30 минут",
		"Лучшие моды для классической игры",
		"Что ждать от обновления",
		"Обзор графики и оптимизации",
		"Финал кампании без подсказок",
		"Турнир в киберспорте",
		"История создания серии",
		"Карта секретов и сюжетных триггеров",
	},
	"education": {
		"Производные за 10 минут",
		"Квантовая физика для начинающих",
		"История Второй мировой за 20 минут",
		"Английский для путешествий",
		"Химия элементов и таблица Менделеева",
		"Как устроена ДНК",
		"Введение в логику",
		"Космос и чёрные дыры простыми словами",
		"Эволюция живого мира",
		"Геометрия плоских фигур",
		"Алгоритмы сортировки за 15 минут",
		"Как учиться эффективно",
		"Психология восприятия",
		"Древние цивилизации",
		"Основы статистики",
	},
	"sports": {
		"Утренняя зарядка за 10 минут",
		"План подготовки к марафону",
		"HIIT-тренировка дома",
		"Йога для начинающих",
		"Силовая тренировка спины",
		"Растяжка после бега",
		"Как восстановиться после нагрузки",
		"Кардио для жиросжигания",
		"Тренировка кора без оборудования",
		"Бег по холмам: техника",
		"План тренировок на неделю",
		"Растяжка для офисных работников",
		"Кросс-фит за 20 минут",
		"Силовая на турниках",
		"Спорт без зала: уличный воркаут",
	},
	"news": {
		"Главные новости дня",
		"Мировые события за неделю",
		"Что происходит в экономике",
		"Самые обсуждаемые истории",
		"Аналитика политических решений",
		"Технологии в мире новостей",
		"Большое интервью эксперта",
		"Расследование редакции",
		"Главное за 5 минут",
		"Спортивные итоги недели",
		"Культурные события месяца",
		"Климат и экология сегодня",
		"Прогноз погоды на неделю",
		"Финансовые рынки в обзоре",
		"Цифровая трансформация",
	},
	"entertainment": {
		"Лучший стендап вечера",
		"Скетч о повседневной жизни",
		"Большое интервью со звездой",
		"Обзор нового сериала",
		"Топ-фильмы недели",
		"Книги, которые меняют взгляд",
		"Реакция на популярные клипы",
		"Викторина с подписчиками",
		"Спор о любимом сюжете",
		"Юмористический подкаст",
		"Подборка коротких видео",
		"Разбор финала культового фильма",
		"Лучшие моменты ток-шоу",
		"Игра «Угадай актёра»",
		"Любимые цитаты из фильмов",
	},
	"other": {
		"Рецепт за 10 минут",
		"Лайфхак для дома",
		"Путешествие в горы",
		"Утро в новом городе",
		"Уроки рисования с нуля",
		"Как сделать стильное фото",
		"Подборка крутых идей DIY",
		"Прогулка по старому городу",
		"Один день из жизни автора",
		"Гид по необычным местам",
		"Обзор популярного гаджета",
		"Финансовые советы новичкам",
		"Знакомство с породой собак",
		"Как ухаживать за комнатными растениями",
		"Главные тренды сезона",
	},
}
