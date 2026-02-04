package database

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"xiaozhi/manager/backend/config"
	"xiaozhi/manager/backend/models"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func Init(cfg config.DatabaseConfig) *gorm.DB {
	var db *gorm.DB
	var err error

	storageType := cfg.GetStorageType()

	if storageType == "sqlite" {
		if cfg.SQLite == nil {
			log.Println("SQLite配置为空，将使用fallback模式运行（硬编码用户验证）")
			return nil
		}
		// 确保数据库文件所在目录存在，避免 SQLite 报 unable to open database file
		dir := filepath.Dir(cfg.SQLite.FilePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Printf("创建数据库目录失败 %s: %v", dir, err)
			return nil
		}
		log.Println("使用SQLite数据库:", cfg.SQLite.FilePath)
		db, err = gorm.Open(sqlite.Open(cfg.SQLite.FilePath), &gorm.Config{})
	} else {
		if cfg.MySQL == nil {
			log.Println("MySQL配置为空，将使用fallback模式运行（硬编码用户验证）")
			return nil
		}
		// MySQL 数据库连接
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			cfg.MySQL.Username, cfg.MySQL.Password, cfg.MySQL.Host, cfg.MySQL.Port, cfg.MySQL.Database)
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	}

	if err != nil {
		log.Println("数据库连接失败:", err)
		log.Println("将使用fallback模式运行（硬编码用户验证）")
		return nil
	}

	log.Println("数据库连接成功")

	// 自动迁移数据库表结构
	log.Println("开始自动迁移数据库表结构...")
	err = db.AutoMigrate(
		&models.User{},
		&models.Device{},
		&models.Agent{},
		&models.Config{},
		&models.GlobalRole{},
		&models.ChatMessage{},
		&models.SpeakerGroup{},
		&models.SpeakerSample{},
	)
	if err != nil {
		log.Printf("数据库表结构迁移失败: %v", err)
		log.Println("将使用fallback模式运行（硬编码用户验证）")
		return nil
	}
	log.Println("数据库表结构迁移成功")

	return db
}

func Close(db *gorm.DB) {
	sqlDB, err := db.DB()
	if err != nil {
		log.Println("获取数据库连接失败:", err)
		return
	}
	sqlDB.Close()
}
