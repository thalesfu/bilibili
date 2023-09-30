package main

import (
	"errors"
	"fmt"
	"github.com/spf13/cobra"
	"log"
	"os"
	"path/filepath"
)

var downloadUPerCmd = &cobra.Command{
	Use:   "downloaduper",
	Short: "download uper's videos from videos.yaml",
	Args:  cobra.ExactArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return login()
	},
	Run: func(cmd *cobra.Command, args []string) {
		exitOnError(downloadUPerVideos(args[0]))
	},
}

func init() {
	rootCmd.AddCommand(downloadUPerCmd)
}

func downloadUPerVideos(uper string) error {
	file := getUPerVideosListFileLocation(uper)

	if !PathExists(file) {
		return errors.New(uper + "'s videos.yaml not found")
	}

	content, ok := LoadContent(file)

	if !ok {
		return errors.New("Load " + uper + "'s video.yaml from " + file + " fail.")
	}

	vp, ok := UnmarshalYaml[[]*UpVideoInfo](content)

	if !ok {
		return errors.New("Unmarshal " + uper + "'s video.yaml fail.")
	}

	videos := *vp

	for _, v := range videos {
		if v.Location == "" {
			v, err := setAV(v)
			if err != nil {
				log.Printf("setAV failed: %v\n", err)
				continue
			}
			_, ok, _ := downloadAndMergeVideo(v)
			if ok {
				content = MarshalYaml(videos)
				WriteContent(file, content)
			}
		}
	}

	return nil
}

func downloadAndMergeVideo(v *UpVideoInfo) (*UpVideoInfo, bool, error) {
	folder := getUPerVideosListFolderLocation(v.Author)
	file := filepath.Join(folder, v.Title+"["+v.VideoQuality.String()+","+v.AudioQuality.String()+"].mp4")

	videoTmp, err := os.CreateTemp(folder, "bilibili_video_*.m4s")
	if err != nil {
		log.Printf("CreateTemp videoTmp failed: %v\n", err)
		return v, false, err
	}

	audioTmp, err := os.CreateTemp(outputDir, "bilibili_audio_*.m4s")
	if err != nil {
		log.Printf("CreateTemp audioTmp failed: %v\n", err)
		return v, false, err
	}

	defer func() {
		err := os.Remove(videoTmp.Name())
		if err != nil {
			panic(err)
		}
		err = os.Remove(audioTmp.Name())
		if err != nil {
			panic(err)
		}
	}()

	fmt.Printf("Downloading %s video of %s\n", v.VideoQuality.String(), v.Title)
	if err = downloadMedia("Video", v.VideoURL, videoTmp); err != nil {
		log.Printf("download video failed: %v\n", err)
		return v, false, err
	}
	fmt.Printf("Downloading %s audio of %s\n", v.AudioQuality.String(), v.Title)
	if err = downloadMedia("Audio", v.AudioURL, audioTmp); err != nil {
		log.Printf("download audio failed: %v\n", err)
		return v, false, err
	}
	ins.Start()
	defer ins.Stop()
	f, err := merge(videoTmp.Name(), audioTmp.Name(), file)
	if err != nil {
		log.Printf("merge video and audio failed: %v\n", err)
		return v, false, err
	}

	v.Location = f

	return v, true, nil
}
