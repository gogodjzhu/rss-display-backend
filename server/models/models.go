package models

import (
	"time"
)

type Device struct {
	DeviceID      string    `gorm:"primaryKey;column:device_id" json:"device_id"`
	CurrentItemID *uint     `gorm:"column:current_item_id" json:"current_item_id"`
	LastSeen      time.Time `gorm:"column:last_seen" json:"last_seen"`
	CreatedAt     time.Time `gorm:"column:created_at" json:"created_at"`
	Role          string    `gorm:"column:role;type:text" json:"role"`
	Preference    string    `gorm:"column:preference;type:text" json:"preference"`
}

func (Device) TableName() string {
	return "devices"
}

type Feed struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"column:name" json:"name"`
	URL       string    `gorm:"column:url" json:"url"`
	Enabled   bool      `gorm:"column:enabled" json:"enabled"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func (Feed) TableName() string {
	return "feeds"
}

type Item struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	FeedID      uint       `gorm:"column:feed_id;uniqueIndex:idx_items_feed_url" json:"feed_id"`
	Title       string     `gorm:"column:title" json:"title"`
	URL         string     `gorm:"column:url;uniqueIndex:idx_items_feed_url" json:"url"`
	ImageURL    string     `gorm:"column:image_url" json:"image_url"`
	PublishedAt *time.Time `gorm:"column:published_at" json:"published_at"`
	CreatedAt   time.Time  `gorm:"column:created_at" json:"created_at"`
	Content     string     `gorm:"column:content;type:longtext" json:"content"`
	Abstract    string     `gorm:"column:abstract;type:longtext" json:"abstract"`
}

func (Item) TableName() string {
	return "items"
}

type ItemRating struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ItemID    uint      `gorm:"column:item_id;index" json:"item_id"`
	DeviceID  string    `gorm:"column:device_id" json:"device_id"`
	Rating    int       `gorm:"column:rating" json:"rating"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func (ItemRating) TableName() string {
	return "item_ratings"
}

type ItemRead struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ItemID    uint      `gorm:"column:item_id;index" json:"item_id"`
	DeviceID  string    `gorm:"column:device_id;index" json:"device_id"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func (ItemRead) TableName() string {
	return "item_reads"
}

type ItemShow struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ItemID    uint      `gorm:"column:item_id;index" json:"item_id"`
	DeviceID  string    `gorm:"column:device_id;index" json:"device_id"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func (ItemShow) TableName() string {
	return "item_shows"
}

type Task struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	DeviceID       string     `gorm:"column:device_id;index" json:"device_id"`
	Status         string     `gorm:"column:status" json:"status"`
	TimeRangeStart *time.Time `gorm:"column:time_range_start" json:"time_range_start"`
	TimeRangeEnd   *time.Time `gorm:"column:time_range_end" json:"time_range_end"`
	Level1IDs      string     `gorm:"column:level1_ids;type:text" json:"level1_ids"`
	Level2IDs      string     `gorm:"column:level2_ids;type:text" json:"level2_ids"`
	Report         string     `gorm:"column:report;type:longtext" json:"report"`
	Error          string     `gorm:"column:error;type:text" json:"error"`
	CreatedAt      time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at" json:"updated_at"`
	CompletedAt    *time.Time `gorm:"column:completed_at" json:"completed_at"`
}

func (Task) TableName() string {
	return "tasks"
}
