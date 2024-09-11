package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/bwmarrin/discordgo"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var (
    Token      string
    db         *gorm.DB
    s *discordgo.Session
)

type Todo struct {
    ID        uint      `gorm:"column:id; primaryKey; autoIncrement"`
    UserID    string    `gorm:"column:user_id; type:varchar(20); not null; index:idx_user_id"`
    GuildID   string    `gorm:"column:guild_id; type:varchar(20); not null; index:idx_guild_id"`
    Task      string    `gorm:"column:task; type:text; not null"`
    IsDone    bool      `gorm:"column:is_done; not null; default:false"`
    CreatedAt time.Time `gorm:"column:created_at; type:datetime; default:CURRENT_TIMESTAMP; index:idx_created_at"`
}

type Tabler interface {
	TableName() string
}

func (Todo) TableName() string {
	return "Todo"
}

func main() {
    var err error

	dsn := fmt.Sprintf(
		"%v:%v@tcp(%v:%v)/%v?charset=utf8mb4&parseTime=True&loc=Local",
		os.Getenv("MYSQL_USER"),
		os.Getenv("MYSQL_PASSWORD"),
		os.Getenv("MYSQL_HOST"),
		os.Getenv("MYSQL_PORT"),
		os.Getenv("MYSQL_DATABASE"),
	)
    db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
        PrepareStmt: true,
    })
    if err != nil {
        log.Fatalf("データベースへの接続に失敗しました: %v", err)
    }

	Token = os.Getenv("DISCORD_TOKEN")

    s, err = discordgo.New("Bot " + Token)
    if err != nil {
        fmt.Println("Discord セッションの作成中にエラーが発生しました,", err)
        return
    }

    err = s.Open()

	// スラッシュコマンドの登録
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "todo",
			Description: "タスクを追加します",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "task",
					Description: "追加するタスク",
					Required:    true,
				},
			},
		},
		{
			Name:        "todos",
			Description: "タスクを一覧表示します",
		},
		{
			Name:        "done",
			Description: "タスクを完了としてマークします",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "task_id",
					Description: "完了させるタスクのID",
					Required:    true,
				},
			},
		},
	}

	
	registeredCommands := make([]*discordgo.ApplicationCommand, len(commands))
	for i, v := range commands {
		for _, g := range s.State.Guilds {
			cmd, err := s.ApplicationCommandCreate(s.State.User.ID, g.ID, v)
			if err != nil {
				log.Panicf("Cannot create '%v' command: %v", v.Name, err)
			}
			registeredCommands[i] = cmd
		}
	}

	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
			h(s, i)
		}
	})
	
	/*
	log.Println("Removing commands...")

	for _, v := range registeredCommands {
		for _, g := range s.State.Guilds {
			err := s.ApplicationCommandDelete(s.State.User.ID, g.ID, v.ID)
			if err != nil {
				log.Panicf("Cannot delete '%v' command: %v", v.Name, err)
			}
		}
	}
	*/
	

    if err != nil {
        fmt.Println("接続オープン中にエラーが発生しました,", err)
        return
    }

    fmt.Println("ボットが起動しました。CTRL+C で終了します。")
    select {}
}

var commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
    "todo": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
        cmdData := i.ApplicationCommandData()
        if len(cmdData.Options) > 0 {
            task := cmdData.Options[0].StringValue()
            guildID := i.GuildID
			userID := i.Member.User.ID
			addTodo(userID, guildID, task)
            s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
                Type: discordgo.InteractionResponseChannelMessageWithSource,
                Data: &discordgo.InteractionResponseData{
                    Content: "タスクが追加されました！",
                },
            })
        }
    },
    "todos": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
        guildID := i.GuildID
		userID := i.Member.User.ID
        todos := getTodos(userID, guildID)
        var response string
        for _, todo := range todos {
            status := "未完了"
            if todo.IsDone {
                status = "完了"
            }
			response += fmt.Sprintf("ID: %d - タスク: %s - 追加日: %s - %s\n", todo.ID, todo.Task, todo.CreatedAt.Local().Format("2006/01/02 15:04:05"), status)
        }
        if response == "" {
            response = "タスクはありません。"
        }
        s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
            Type: discordgo.InteractionResponseChannelMessageWithSource,
            Data: &discordgo.InteractionResponseData{
                Content: response,
            },
        })
    },
    "done": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
        cmdData := i.ApplicationCommandData()
        if len(cmdData.Options) > 0 {
            todoIDStr := cmdData.Options[0].StringValue()
			guildID := i.GuildID
			userID := i.Member.User.ID
            todoID, err := strconv.ParseUint(todoIDStr, 10, 32)
            if err != nil {
                s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
                    Type: discordgo.InteractionResponseChannelMessageWithSource,
                    Data: &discordgo.InteractionResponseData{
                        Content: "無効なタスクIDです。",
                    },
                })
                return
            }
			todo := getTodo(userID, guildID, uint(todoID))
			if todo == (Todo{}) {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
                    Type: discordgo.InteractionResponseChannelMessageWithSource,
                    Data: &discordgo.InteractionResponseData{
                        Content: "タスクが見つかりませんでした。",
                    },
                })
				return
			}
			if todo.IsDone {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "このタスクはすでに完了しています。",
					},
				})
				return
			}
            if markTaskAsDone(userID, guildID, uint(todoID)) {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "タスクが完了としてマークされました。",
					},
				})
			}
        }
    },
}

func addTodo(userID string, guildID string, task string) {
    db.Create(&Todo{UserID: userID, GuildID: guildID, Task: task, CreatedAt: time.Now()})
}

func getTodos(userID string, guildID string) []Todo {
    var todos []Todo
    db.Where("user_id = ? AND guild_id = ?", userID, guildID).Find(&todos)
    return todos
}

func getTodo(userID string, guildID string, todoID uint) Todo {
	var todo Todo
	db.Where("user_id = ? AND guild_id = ? AND id = ?", userID, guildID, todoID).First(&todo)
	return todo
}

func markTaskAsDone(userID string, guildID string, todoID uint) bool {
    var todo Todo
	db.Where("user_id = ? AND guild_id = ? AND id = ?", userID, guildID, todoID).First(&todo)
	if todo == (Todo{}) {
		return false
	}
	todo.IsDone = true
	db.Save(&todo)
	return true
}
