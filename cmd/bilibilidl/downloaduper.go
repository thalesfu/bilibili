package main

import (
	"fmt"
	"github.com/spf13/cobra"
	"log"
	"os"
	"path"
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

	videos := AllVideos[uper]

	for _, v := range videos {
		if v.Location == "" {
			v, err := setAV(v)
			if err != nil {
				log.Printf("setAV failed: %v\n", err)
				continue
			}
			ok := false
			if v.VideoQuality != 0 {
				_, ok, _ = downloadAndMergeVideo(v)
			} else {
				_, ok, _ = downloadVideo(v)
			}
			if ok {
				content := MarshalYaml(videos)
				WriteContent(getUPerVideosListFileLocation(uper), content)
			}
		}
	}

	return nil
}

func downloadVideo(v *UpVideoInfo) (*UpVideoInfo, bool, error) {
	folder := path.Join(getUPerVideosListFolderLocation(v.Author), v.Title)
	err := os.MkdirAll(folder, os.ModePerm)
	if err != nil {
		log.Printf("MkdirAll %s failed: %v\n", folder, err)
		return v, false, err
	}
	fileName := v.Part + ".mp4"
	file := filepath.Join(folder, fileName)

	writer, err := getDownloadDestFile(folder, file)
	if err != nil {
		return nil, false, err
	}
	defer func(writer *os.File) {
		err := writer.Close()
		if err != nil {
			panic(err)
		}
	}(writer)

	fmt.Printf("Download then video of %s directly.\n", v.Title)
	err = downloadMedia("Video", v.DownloadURL, writer)
	if err != nil {
		return nil, false, err
	}

	return v, true, nil
}

func downloadAndMergeVideo(v *UpVideoInfo) (*UpVideoInfo, bool, error) {
	folder := path.Join(getUPerVideosListFolderLocation(v.Author), v.Title)
	err := os.MkdirAll(folder, os.ModePerm)
	if err != nil {
		log.Printf("MkdirAll %s failed: %v\n", folder, err)
		return v, false, err
	}
	file := filepath.Join(folder, v.Part+"["+v.VideoQuality.String()+","+v.AudioQuality.String()+"].mp4")

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
