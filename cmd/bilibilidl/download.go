package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"sort"

	bilibili "github.com/misssonder/bilibili/pkg/client"
	"github.com/misssonder/bilibili/pkg/errors"
	"github.com/misssonder/bilibili/pkg/video"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/vbauerster/mpb/v5"
	"github.com/vbauerster/mpb/v5/decor"
	"github.com/xyctruth/stream"
)

var (
	outputFile string
	outputDir  string
)

var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download bilibili video through url/BVID/AVID.",
	Args:  cobra.ExactArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if err := checkDir(); err != nil {
			return err
		}
		if err := checkOutputFormat(); err != nil {
			return err
		}
		return login()
	},
	Run: func(cmd *cobra.Command, args []string) {
		id, err := video.ExtractBvID(args[0])
		if err != nil {
			exitOnError(err)
		}
		exitOnError(download(id))
	},
}

func init() {
	rootCmd.AddCommand(downloadCmd)
	downloadCmd.Flags().StringVarP(&outputFile, "filename", "o", "", "The output file.")
	downloadCmd.Flags().StringVarP(&outputDir, "directory", "d", ".", "The output directory.")
}

func selectVideoInfo(info *VideoInfo) (Page, error) {
	pages := info.Pages
	rows := make([]string, 0, len(pages))
	for i, page := range pages {
		rows = append(rows, fmt.Sprintf("%d. %s", i+1, page.Part))
	}
	selectedPage, err := selectList("Please select page", rows)
	if err != nil {
		return Page{}, err
	}
	return pages[selectedPage], nil
}

func selectSeasonInfo(info *SeasonInfo) (Episode, error) {
	episodes := info.Episodes
	rows := make([]string, 0, len(episodes))
	for i, episode := range episodes {
		rows = append(rows, fmt.Sprintf("%d. %s", i+1, episode.Title))
	}
	selectedPage, err := selectList("Please select episode", rows)
	if err != nil {
		return Episode{}, err
	}
	return episodes[selectedPage], nil
}

func selectFormat() (bilibili.Fnval, error) {
	formats := map[string]bilibili.Fnval{
		"MP4":  bilibili.FnvalMP4,
		"DASH": bilibili.FnvalDash,
	}
	rows := []string{
		"MP4",
		"DASH",
	}
	format, err := selectList("Please select video format", rows)
	if err != nil {
		return 0, err
	}
	return formats[rows[format]], nil
}

func selectMediaQuality(title string, qns []bilibili.Qn) (bilibili.Qn, error) {
	var marshalQns = func(qns []bilibili.Qn) []bilibili.Qn {
		qns = stream.NewSliceByOrdered(qns).Distinct().ToSlice()
		tmp := make([]int, 0, len(qns))
		for _, qn := range qns {
			tmp = append(tmp, int(qn))
		}
		sort.Ints(tmp)
		for i, t := range tmp {
			qns[i] = bilibili.Qn(t)
		}
		return qns
	}
	qns = marshalQns(qns)
	rows := make([]string, 0, len(qns))
	for _, qn := range qns {
		rows = append(rows, qn.String())
	}
	selected, err := selectList(title, rows)
	if err != nil {
		return 0, err
	}
	return qns[selected], nil
}

func download(id string) error {
	var (
		bvID  string
		cid   int64
		title string
	)
	if video.IsSSID(id) || video.IsEpID(id) {
		info, err := getSeasonInfo(id)
		if err != nil {
			return err
		}
		episode, err := selectSeasonInfo(info)
		if err != nil {
			return err
		}
		cid = episode.CID
		bvID = episode.BvID
		title = episode.Title
	} else {
		info, err := getVideoInfo(id)
		if err != nil {
			return err
		}
		page, err := selectVideoInfo(info)
		if err != nil {
			return err
		}
		cid = page.CID
		bvID = id
		title = info.Title
	}

	format, err := selectFormat()
	if err != nil {
		return err
	}

	if len(outputFile) == 0 {
		outputFile = fmt.Sprintf("%s.mp4", title)
	}

	switch format {
	case bilibili.FnvalMP4:
		playUrlResp, err := client.PlayUrl(bvID, cid, bilibili.Qn4k, format)
		if err != nil {
			return err
		}

		writer, err := getDownloadDestFile(outputDir, outputFile)
		if err != nil {
			return err
		}
		defer writer.Close()

		return downloadMedia("Video", playUrlResp.Data.Durl[0].URL, writer)
	case bilibili.FnvalDash:
		if err = checkFFmpeg(); err != nil {
			return err
		}
		playUrlResp, err := client.PlayUrl(bvID, cid, 0, format)
		if err != nil {
			return err
		}
		var (
			selectedVideoQuality bilibili.Qn
			selectedAudioQuality bilibili.Qn
			videoTmp             *os.File
			audioTmp             *os.File
		)
		{
			videoQualities := make([]bilibili.Qn, 0, len(playUrlResp.Data.Dash.Video))
			videoTmp, err = os.CreateTemp(outputDir, "bilibili_video_*.m4s")
			if err != nil {
				return err
			}
			defer os.Remove(videoTmp.Name())
			for _, video := range playUrlResp.Data.Dash.Video {
				videoQualities = append(videoQualities, bilibili.Qn(video.ID))
			}
			selectedVideoQuality, err = selectMediaQuality("Please select video quality", videoQualities)
			if err != nil {
				return err
			}
		}
		{
			audioQualities := make([]bilibili.Qn, 0, len(playUrlResp.Data.Dash.Audio))
			audioTmp, err = os.CreateTemp(outputDir, "bilibili_audio_*.m4s")
			if err != nil {
				return err
			}
			defer os.Remove(audioTmp.Name())
			for _, audio := range playUrlResp.Data.Dash.Audio {
				audioQualities = append(audioQualities, bilibili.Qn(audio.ID))
			}
			selectedAudioQuality, err = selectMediaQuality("Please select audio quality", audioQualities)
			if err != nil {
				return err
			}
		}
		if err = downloadMedia("Video", chooseMediaUrl(playUrlResp, selectedVideoQuality), videoTmp); err != nil {
			return err
		}
		if err = downloadMedia("Audio", chooseMediaUrl(playUrlResp, selectedAudioQuality), audioTmp); err != nil {
			return err
		}
		ins.Start()
		defer ins.Stop()
		_, err = merge(videoTmp.Name(), audioTmp.Name(), path.Join(outputDir, outputFile))

		return err
	}
	return nil
}

func chooseMediaUrl(playUrlResp *bilibili.PlayUrlResp, qn bilibili.Qn) string {
	if qn > 2048 {
		for _, audio := range playUrlResp.Data.Dash.Audio {
			if audio.ID == int(qn) {
				return audio.BaseURL
			}
		}
		return playUrlResp.Data.Dash.Audio[0].BaseURL
	} else {
		for _, video := range playUrlResp.Data.Dash.Video {
			if video.ID == int(qn) {
				return video.BaseURL
			}
		}
		return playUrlResp.Data.Dash.Video[0].BaseURL
	}

}

func merge(video, audio, output string) (string, error) {
	cmd := exec.Command("ffmpeg", "-y",
		"-i", video,
		"-i", audio,
		"-c", "copy", // Just copy without re-encoding
		"-shortest", // Finish encoding when the shortest input stream ends
		output,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	var stdout bytes.Buffer
	cmd.Stderr = &stdout
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		panic(err)
	}

	if err := cmd.Start(); err != nil {
		fmt.Printf("%s\n", stderr.String())
		return "", err
	}

	scanner := bufio.NewScanner(stdoutPipe)
	for scanner.Scan() {
		fmt.Println(scanner.Text())
	}

	err = cmd.Wait()
	if err != nil {
		fmt.Printf("%s\n", stderr.String())
		return "", err
	}

	fmt.Printf("%s\n", stdout.String())
	fmt.Printf("%s is merged from video %s and audio %s.\n", output, video, audio)
	return output, nil
}

func getDownloadDestFile(dir, f string) (*os.File, error) {
	filePath := path.Join(dir, f)
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	return file, nil
}

func downloadMedia(title, url string, writer io.Writer) error {
	cli := &http.Client{}
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	request.Header.Add("referer", "https://www.bilibili.com")
	resp, err := cli.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.ErrUnexpectedStatusCode(resp.StatusCode)
	}
	progress := mpb.New(mpb.WithWidth(64))
	bar := progress.AddBar(
		resp.ContentLength,

		mpb.PrependDecorators(
			decor.Name(fmt.Sprintf("%s:", title)),
			decor.OnComplete(
				decor.Name("download... "), "done ",
			),
			decor.CountersKibiByte("% .2f / % .2f"),
			decor.Percentage(decor.WCSyncSpace),
		),
		mpb.AppendDecorators(
			decor.EwmaETA(decor.ET_STYLE_GO, 90),
			decor.Name(" | "),
			decor.EwmaSpeed(decor.UnitKiB, "% .2f", 60),
		),
	)
	reader := bar.ProxyReader(resp.Body)
	_, err = io.Copy(writer, reader)
	progress.Wait()
	return err
}

func checkFFmpeg() error {
	logrus.Info("Check ffmpeg is installed....")
	if err := exec.Command("ffmpeg", "-version").Run(); err != nil {
		return fmt.Errorf("please check ffmpegCheck is installed correctly")
	}
	logrus.Info("FFmpeg is installed successfully!")
	return nil
}

func checkDir() error {
	_, err := os.Stat(outputDir)
	return err
}
