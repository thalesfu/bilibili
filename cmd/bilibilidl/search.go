package main

import (
	"fmt"
	bilibili "github.com/misssonder/bilibili/pkg/client"
	"github.com/misssonder/bilibili/pkg/video"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"log"
	"path/filepath"
	"sort"
	"time"
)

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
}

func search(keyword string) error {
	videos, err := client.GetUPerVideos(keyword)

	if err != nil {
		return err
	}

	if len(videos) > 0 {
		upVideos := mapToUPVideos(videos)
		for u, vs := range upVideos {
			fmt.Printf("Uper: %s\n", u)

			for _, v := range vs {
				fmt.Printf("\t%s %s\n", v.Title, v.PublishTime)
			}

			content := MarshalYaml(vs)
			WriteContent(getUPerVideosListFileLocation(u), content)
		}
	}

	return nil
}

func mapToUPVideos(videos map[string]string) map[string][]*UpVideoInfo {
	upVideos := make(map[string][]*UpVideoInfo)
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
			videoInfo := &UpVideoInfo{
				BvID:        info.BvID,
				AID:         info.AID,
				Title:       info.Title,
				Author:      info.Author,
				Duration:    info.Duration,
				PublishTime: info.PublishTime,
				CID:         page.CID,
			}
			videoInfo, err = setAV(videoInfo)
			if err != nil {
				log.Printf("Set AV failed: %v\n", err)
			}
			upVideos[info.Author] = append(upVideos[info.Author], videoInfo)
		}
	}

	for _, vs := range upVideos {
		sort.Slice(vs, func(i, j int) bool {
			return vs[i].PublishTime < vs[j].PublishTime
		})
	}

	return upVideos
}

func setAV(v *UpVideoInfo) (*UpVideoInfo, error) {
	playUrlResp, err := client.PlayUrl(v.BvID, v.CID, 0, bilibili.FnvalDash)
	if err != nil {
		return v, err
	}

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

type UpVideoInfo struct {
	BvID         string        `json:"bvid"`
	AID          int           `json:"aid"`
	Title        string        `json:"title"`
	Author       string        `json:"author"`
	Duration     time.Duration `json:"duration"`
	PublishTime  string        `json:"pubdate"`
	CID          int64         `json:"cid"`
	VideoQuality bilibili.Qn   `json:"video_quality"`
	AudioQuality bilibili.Qn   `json:"audio_quality"`
	VideoURL     string        `json:"video_url"`
	AudioURL     string        `json:"audio_url"`
	Location     string        `json:"location"`
}

func getUPerVideosListFileLocation(uper string) string {
	return filepath.Join(getUPerVideosListFolderLocation(uper), "videos.yaml")
}

func getUPerVideosListFolderLocation(uper string) string {
	return filepath.Join(getVideoLocation(), uper)
}
