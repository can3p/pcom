package media

import (
	"fmt"
	"html/template"
	"regexp"
)

var youtubeRE *regexp.Regexp = regexp.MustCompile(`^https://(?:www.)?(?:youtube(?:-nocookie)?\.com\/(?:[^\/]+\/.+\/|(?:v|e(?:mbed)?)\/|.*[?&]v=)|youtu\.be\/)([^"&?\/ ]{11})$`)

type YoutubeLink struct {
	ID string
}

func (l *YoutubeLink) Key() string {
	return fmt.Sprintf("youtube: %s", l.ID)
}

func (l *YoutubeLink) EmbedCode() template.HTML {
	return template.HTML(
		fmt.Sprintf(`<lite-youtube videoid="%s" playlabel="Play Video" nocookie></lite-youtube>`, l.ID),
	)
}

func (l *YoutubeLink) URL() string {
	return fmt.Sprintf(`https://www.youtube.com/watch?v=%s`, l.ID)
}

type YoutubeParser struct {
}

func (p *YoutubeParser) Parse(url string) MediaLink {
	groups := youtubeRE.FindStringSubmatch(url)

	if len(groups) == 2 {
		return &YoutubeLink{
			ID: groups[1],
		}
	}
	return nil
}
