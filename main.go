package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"

	bot "github.com/meinside/telegram-bot-go"
	witai "github.com/meinside/wit.ai-go"
)

const (
	TelegramApiToken = ""
	WitaiApiToken    = ""

	MonitorIntervalSeconds = 1
	Verbose                = false // XXX - set this to 'true' for verbose messages

	TempDir       = "/tmp"                  // XXX - Edit this value to yours
	FfmpegBinPath = "/usr/local/bin/ffmpeg" // XXX - Edit this value to yours
)

// Download given url and return the downloaded path
func downloadFile(url string) (filepath string, err error) {
	log.Printf("> downloading voice file: %s\n", url)

	var file *os.File
	if file, err = ioutil.TempFile(TempDir, "downloaded_"); err == nil {
		filepath = file.Name()

		defer file.Close()

		var response *http.Response
		if response, err = http.Get(url); err == nil {
			defer response.Body.Close()

			if _, err = io.Copy(file, response.Body); err == nil {
				log.Printf("> finished downloading voice file: %s\n", filepath)
			}
		}
	}

	return filepath, err
}

// Convert .ogg to .mp3 (using ffmpeg)
//
// NOTE: wit.ai doesn't support stereo sound for now
// (https://wit.ai/docs/http/20160516#post--speech-link)
func oggToMp3(oggFilepath string) (mp3Filepath string, err error) {
	mp3Filepath = fmt.Sprintf("%s.mp3", oggFilepath)

	// $ ffmpeg -i input.ogg -ac 1 output.mp3
	params := []string{"-i", oggFilepath, "-ac", "1", mp3Filepath}
	cmd := exec.Command("ffmpeg", params...)

	if _, err = cmd.CombinedOutput(); err != nil {
		mp3Filepath = ""
	}

	return mp3Filepath, err
}

// Download a file from given url and convert it to a text.
//
// Downloaded or converted files will be deleted automatically.
func speechToText(w *witai.Client, fileUrl string) (text string, err error) {
	var oggFilepath, mp3Filepath string

	// download .ogg,
	if oggFilepath, err = downloadFile(fileUrl); err == nil {
		// .ogg => .mp3,
		if mp3Filepath, err = oggToMp3(oggFilepath); err == nil {
			// .mp3 => text
			if result, err := w.QuerySpeechMp3(mp3Filepath, nil, "", "", 1); err == nil {
				log.Printf("> analyzed speech result: %+v\n", result)

				if result.Text != nil {
					text = fmt.Sprintf("\"%s\"", *result.Text)

					/*
					   // traverse for more info
					   sessionId := "01234567890abcdef"
					   if results, err := w.ConverseAll(sessionId, *result.Text, nil); err == nil {
					       for i, r := range results {
					           log.Printf("> converse[%d] result: %v\n", i, r)
					       }
					   } else {
					       log.Printf("failed to converse: %s\n", err)
					   }
					*/
				}
			}

			// delete converted file
			if err = os.Remove(mp3Filepath); err != nil {
				log.Printf("*** failed to delete converted file: %s\n", mp3Filepath)
			}
		} else {
			log.Printf("*** failed to convert .ogg to .mp3: %s\n", err)
		}

		// delete downloaded file
		if err = os.Remove(oggFilepath); err != nil {
			log.Printf("*** failed to delete downloaded file: %s\n", oggFilepath)
		}
	}

	return text, err
}

func main() {
	b := bot.NewClient(TelegramApiToken)
	b.Verbose = Verbose

	w := witai.NewClient(WitaiApiToken)
	w.Verbose = Verbose

	if unhooked := b.DeleteWebhook(); unhooked.Ok { // delete webhook
		// wait for new updates
		b.StartMonitoringUpdates(0, MonitorIntervalSeconds, func(b *bot.Bot, u bot.Update, err error) {
			if err == nil && u.HasMessage() {
				b.SendChatAction(u.Message.Chat.Id, bot.ChatActionTyping) // typing...

				if u.Message.HasVoice() { // when voice is received,
					if sent := b.GetFile(u.Message.Voice.FileId); sent.Ok {
						if message, err := speechToText(w, b.GetFileUrl(*sent.Result)); err == nil {
							if len(message) <= 0 {
								message = "Failed to analyze your voice."
							}
							if sent := b.SendMessage(u.Message.Chat.Id, &message, map[string]interface{}{}); !sent.Ok {
								log.Printf("*** failed to send message: %s\n", *sent.Description)
							}
						} else {

							message := fmt.Sprintf("Failed to analyze your voice: %s", err)
							if sent := b.SendMessage(u.Message.Chat.Id, &message, map[string]interface{}{}); !sent.Ok {
								log.Printf("*** failed to send message: %s\n", *sent.Description)
							}
						}
					}
				} else { // otherwise,
					message := "Let me hear your voice."
					if sent := b.SendMessage(u.Message.Chat.Id, &message, map[string]interface{}{}); !sent.Ok {
						log.Printf("*** failed to send message: %s\n", *sent.Description)
					}
				}
			}
		})
	} else {
		panic("failed to delete webhook")
	}
}
