package models

import "time"

type User struct {
	ID           string     `json:"id"`
	Username     string     `json:"username"`
	DisplayName  string     `json:"displayName"`
	Email        string     `json:"email"`
	AvatarURL    string     `json:"avatarUrl"`
	Bio          string     `json:"bio"`
	Role         string     `json:"role"`
	Blocked      bool       `json:"blocked"`
	CreatedAt    time.Time  `json:"createdAt"`
	Premium      bool       `json:"premium"`
	PremiumUntil *time.Time `json:"premiumUntil,omitempty"`
}

type Channel struct {
	ID               string    `json:"id"`
	OwnerID          string    `json:"ownerId"`
	Name             string    `json:"name"`
	Handle           string    `json:"handle"`
	Description      string    `json:"description"`
	AvatarURL        string    `json:"avatarUrl"`
	BannerURL        string    `json:"bannerUrl"`
	SubscribersCount int64     `json:"subscribersCount"`
	CreatedAt        time.Time `json:"createdAt"`
	Balance          int64     `json:"balance"`
	TotalEarned      int64     `json:"totalEarned"`
}

type Ad struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	VideoURL    string    `json:"videoUrl"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Transaction struct {
	ID          string    `json:"id"`
	UserID      *string   `json:"userId,omitempty"`
	ChannelID   *string   `json:"channelId,omitempty"`
	Type        string    `json:"type"`
	Amount      int64     `json:"amount"`
	Status      string    `json:"status"`
	Description string    `json:"description"`
	CardLast4   *string   `json:"cardLast4,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Video struct {
	ID               string    `json:"id"`
	ChannelID        string    `json:"channelId"`
	Title            string    `json:"title"`
	Description      string    `json:"description"`
	ThumbnailURL     string    `json:"thumbnailUrl"`
	VideoURL         string    `json:"videoUrl"`
	DurationSec      int       `json:"durationSec"`
	Views            int64     `json:"views"`
	Likes            int64     `json:"likes"`
	Dislikes         int64     `json:"dislikes"`
	Category         string    `json:"category"`
	Visibility       string    `json:"visibility"`
	Tags             []string  `json:"tags"`
	UploadedAt       time.Time `json:"uploadedAt"`
	ChannelName      string    `json:"channelName,omitempty"`
	ChannelAvatar    string    `json:"channelAvatar,omitempty"`
	TranscriptStatus string    `json:"transcriptStatus"`
	OriginalLanguage *string   `json:"originalLanguage,omitempty"`
	Chapters         []Chapter `json:"chapters"`
	// Ремикс / атрибуция (см. миграцию 009).
	AllowRemix        bool    `json:"allowRemix"`
	SourceVideoID     *string `json:"sourceVideoId,omitempty"`
	SourceChannelID   *string `json:"sourceChannelId,omitempty"`
	SourceChannelName *string `json:"sourceChannelName,omitempty"`
}

// Статусы процесса транскрипции (общие для уровня видео и каждого языка).
const (
	TranscriptNotStarted  = "NOT_STARTED"
	TranscriptProcessing  = "PROCESSING"
	TranscriptTranslating = "TRANSLATING"
	TranscriptCompleted   = "COMPLETED"
	TranscriptFailed      = "FAILED"
)

// Chapter — таймкод-глава, автоматически генерируется из сегментов расшифровки.
type Chapter struct {
	Start float64 `json:"start"`
	Title string  `json:"title"`
}

// TranscriptSegment — один сегмент таймкодированной расшифровки.
// start/end — секунды от начала видео, общие для всех языков.
type TranscriptSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

// Transcript — расшифровка/перевод на конкретный язык.
type Transcript struct {
	VideoID    string              `json:"videoId"`
	Language   string              `json:"language"`
	IsOriginal bool                `json:"isOriginal"`
	Status     string              `json:"status"`
	FullText   string              `json:"fullText"`
	Segments   []TranscriptSegment `json:"segments"`
	VTTUrl     string              `json:"vttUrl,omitempty"`
	Error      string              `json:"error,omitempty"`
	UpdatedAt  time.Time           `json:"updatedAt"`
}

type Comment struct {
	ID        string    `json:"id"`
	VideoID   string    `json:"videoId"`
	AuthorID  string    `json:"authorId"`
	Text      string    `json:"text"`
	Likes     int       `json:"likes"`
	CreatedAt time.Time `json:"createdAt"`
}

type Subscription struct {
	ID            string    `json:"id"`
	SubscriberID  string    `json:"subscriberId"`
	ChannelID     string    `json:"channelId"`
	SubscribedAt  time.Time `json:"subscribedAt"`
}

type StatsPoint struct {
	Date        string `json:"date"`
	Views       int64  `json:"views"`
	Subscribers int64  `json:"subscribers"`
	Likes       int64  `json:"likes"`
	Dislikes    int64  `json:"dislikes"`
}

type ChannelStats struct {
	ChannelID            string       `json:"channelId"`
	Points               []StatsPoint `json:"points"`
	SubscribedRecently   []string     `json:"subscribedRecently"`
	UnsubscribedRecently []string     `json:"unsubscribedRecently"`
	TotalViews           int64        `json:"totalViews"`
	TotalLikes           int64        `json:"totalLikes"`
	TotalDislikes        int64        `json:"totalDislikes"`
	TotalSubscribers     int64        `json:"totalSubscribers"`
}

type PlatformStats struct {
	TotalUsers     int64                `json:"totalUsers"`
	TotalChannels  int64                `json:"totalChannels"`
	TotalVideos    int64                `json:"totalVideos"`
	TotalViews     int64                `json:"totalViews"`
	TotalComments  int64                `json:"totalComments"`
	DailyActive    []DailyActivityPoint `json:"dailyActive"`
}

type DailyActivityPoint struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}
