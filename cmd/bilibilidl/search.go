package main

import (
	"fmt"
	bilibili "github.com/misssonder/bilibili/pkg/client"
	"github.com/misssonder/bilibili/pkg/video"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"log"
	"os"
	"path/filepath"
	"time"
)

var AllVideos map[string][]*UpVideoInfo

var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "search keyword and generate videos.yaml",
	Args:  cobra.ExactArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return login()
	},
	Run: func(cmd *cobra.Command, args []string) {
		exitOnError(search(args[0]))
	},
}

func init() {
	rootCmd.AddCommand(searchCmd)
	initAllVideos()
}

func initAllVideos() {
	folder := getVideoLocation()
	AllVideos = make(map[string][]*UpVideoInfo)

	filepath.Walk(folder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || info.Name() != "videos.yaml" {
			return nil
		}

		content, ok := LoadContent(path)
		if !ok {
			return nil
		}

		vp, ok := UnmarshalYaml[[]*UpVideoInfo](content)
		if !ok {
			return nil
		}

		videos := *vp

		AllVideos[videos[0].Author] = videos
		return nil
	})
}

func search(keyword string) error {
	videos, err := client.GetUPerVideos(keyword)

	if err != nil {
		return err
	}

	if len(videos) > 0 {
		updateVideos(videos)
	}

	return nil
}

func updateVideos(videos []string) {
	for _, vurl := range videos {
		bvID, err := video.ExtractBvID(vurl)
		if err != nil {
			log.Printf("Extract bvID failed: %v\n", err)
		}

		info, err := getVideoInfo(bvID)
		if err != nil {
			log.Printf("Get video info failed: %v\n", err)
		}

		for _, page := range info.Pages {
			_, ok := findVideo(info, page)
			if ok {
				continue
			}

			videoInfo := &UpVideoInfo{
				BvID:        info.BvID,
				AID:         info.AID,
				Title:       info.Title,
				Part:        page.Part,
				Author:      info.Author,
				Duration:    info.Duration,
				PublishTime: info.PublishTime,
				CID:         page.CID,
			}
			videoInfo, err = setAV(videoInfo)
			if err != nil {
				log.Printf("Set AV failed: %v\n", err)
			}
			AllVideos[info.Author] = append(AllVideos[info.Author], videoInfo)

			content := MarshalYaml(AllVideos[info.Author])
			WriteContent(getUPerVideosListFileLocation(info.Author), content)

			fmt.Printf("Add %s's video: %s %s %s\n", videoInfo.Author, videoInfo.Title, videoInfo.Part, videoInfo.PublishTime)

		}
	}
}

func findVideo(v *VideoInfo, p Page) (*UpVideoInfo, bool) {
	vs := AllVideos[v.Author]
	for _, v := range vs {
		if v.CID == p.CID {
			return v, true
		}
	}

	return nil, false
}

func setAV(v *UpVideoInfo) (*UpVideoInfo, error) {
	playUrlResp, err := client.PlayUrl(v.BvID, v.CID, 0, bilibili.FnvalDash)
	if err != nil {
		return v, err
	}

	if len(playUrlResp.Data.Dash.Video) > 0 && len(playUrlResp.Data.Dash.Audio) > 0 {
		maxVideo := lo.MaxBy(playUrlResp.Data.Dash.Video, func(item, max bilibili.DashVideo) bool {
			return item.ID > max.ID
		})

		v.VideoQuality = bilibili.Qn(maxVideo.ID)
		v.VideoURL = chooseMediaUrl(playUrlResp, v.VideoQuality)

		maxAudio := lo.MaxBy(playUrlResp.Data.Dash.Audio, func(item, max bilibili.DashAudio) bool {
			return item.ID > max.ID
		})

		v.AudioQuality = bilibili.Qn(maxAudio.ID)
		v.AudioURL = chooseMediaUrl(playUrlResp, v.AudioQuality)

		return v, nil
	}

	if len(playUrlResp.Data.Durl) > 0 && playUrlResp.Data.Durl[0].URL != "" {
		v.DownloadURL = playUrlResp.Data.Durl[0].URL
	}

	return v, nil
}

type UpVideoInfo struct {
	BvID         string        `json:"bvid"`
	AID          int           `json:"aid"`
	Title        string        `json:"title"`
	Part         string        `json:"part"`
	Author       string        `json:"author"`
	Duration     time.Duration `json:"duration"`
	PublishTime  string        `json:"pubdate"`
	CID          int64         `json:"cid"`
	VideoQuality bilibili.Qn   `json:"video_quality"`
	AudioQuality bilibili.Qn   `json:"audio_quality"`
	VideoURL     string        `json:"video_url"`
	AudioURL     string        `json:"audio_url"`
	DownloadURL  string        `json:"download_url"`
	Location     string        `json:"location"`
}

func getUPerVideosListFileLocation(uper string) string {
	return filepath.Join(getUPerVideosListFolderLocation(uper), "videos.yaml")
}

func getUPerVideosListFolderLocation(uper string) string {
	return filepath.Join(getVideoLocation(), uper)
}
