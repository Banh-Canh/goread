package backend

import (
	"errors"
	"log"

	"github.com/TypicalAM/goread/internal/backend/cache"
	"github.com/TypicalAM/goread/internal/backend/rss"
	"github.com/TypicalAM/goread/internal/ui/simplelist"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mmcdole/gofeed"
)

// Backend provides a way of fetching data from the cache and the RSS feed.
type Backend struct {
	Rss        *rss.Rss
	Cache      *cache.Cache
	ReadStatus *cache.ReadStatus
}

// New creates a new backend and its components.
func New(urlPath, cachePath string, resetCache bool) (*Backend, error) {
	log.Println("Creating new backend")
	store, err := cache.New(cachePath)
	if err != nil {
		return nil, err
	}

	readStatus, err := cache.NewReadStatus("") // TODO: Make this configurable
	if err != nil {
		return nil, err
	}

	if !resetCache {
		if err = store.Load(); err != nil {
			log.Println("Cache load failed: ", err)
		}

		if err = readStatus.Load(); err != nil {
			log.Println("Read status load failed: ", err)
		}
	}

	rss, err := rss.New(urlPath)
	if err != nil {
		return nil, err
	}

	if err = rss.Load(); err != nil {
		log.Println("Rss load failed: ", err)
	}

	return &Backend{rss, store, readStatus}, nil
}

// FetchCategories gets the categories.
func (b Backend) FetchCategories(_ string) tea.Cmd {
	return func() tea.Msg {
		names, descs := b.Rss.GetCategories()

		items := make([]list.Item, len(names))
		for i := range names {
			items[i] = simplelist.NewItem(names[i], descs[i])
		}

		return FetchSuccessMsg{Items: items}
	}
}

// FetchFeeds gets the feeds from a category.
func (b Backend) FetchFeeds(catname string) tea.Cmd {
	return func() tea.Msg {
		names, urls, err := b.Rss.GetFeeds(catname)
		if err != nil {
			return FetchErrorMsg{err, "Error while trying to get feeds"}
		}

		items := make([]list.Item, len(names))
		for i := range names {
			items[i] = simplelist.NewItem(names[i], urls[i])
		}

		return FetchSuccessMsg{items}
	}
}

// FetchArticles gets the articles from a feed.
func (b Backend) FetchArticles(feedName string) tea.Cmd {
	return func() tea.Msg {
		url, err := b.Rss.GetFeedURL(feedName)
		if err != nil {
			return FetchErrorMsg{err, "Error while trying to get the article url"}
		}

		items, err := b.Cache.GetArticles(url)
		if err != nil {
			return FetchErrorMsg{err, "Error while fetching the article"}
		}

		return b.articlesToSuccessMsg(items)
	}
}

// FetchAllArticles gets all the articles from all the feeds.
func (b Backend) FetchAllArticles(_ string) tea.Cmd {
	return func() tea.Msg {
		return b.articlesToSuccessMsg(b.Cache.GetArticlesBulk(b.Rss.GetAllURLs()))
	}
}

// FetchDownloaded gets the downloaded articles.
func (b Backend) FetchDownloadedArticles(_ string) tea.Cmd {
	return func() tea.Msg {
		return b.articlesToSuccessMsg(b.Cache.GetDownloaded())
	}
}

// DownloadItem downloads an article.
func (b Backend) DownloadItem(feedName string, index int) tea.Cmd {
	return func() tea.Msg {
		item, err := b.indexToItem(feedName, index)
		if err != nil {
			return FetchErrorMsg{err, "Error while getting the article"}
		}

		b.Cache.AddToDownloaded(*item)
		return nil
	}
}

// RemoveDownload removes a downloaded article.
func (b Backend) RemoveDownload(index int) error {
	return b.Cache.RemoveFromDownloaded(index)
}

// MarkAsRead marks an article as read.
func (b Backend) MarkAsRead(feedName string, index int) tea.Cmd {
	return func() tea.Msg {
		item, err := b.indexToItem(feedName, index)
		if err != nil {
			return FetchErrorMsg{err, "Error while getting the article"}
		}

		log.Println("Marking as read:", item.Title)
		b.ReadStatus.MarkAsRead(*item)
		return nil
	}
}

// MarkAsUnread marks an article as unread.
func (b Backend) MarkAsUnread(feedName string, index int) tea.Cmd {
	return func() tea.Msg {
		item, err := b.indexToItem(feedName, index)
		if err != nil {
			return FetchErrorMsg{err, "Error while getting the article"}
		}

		log.Println("Marking as unread:", item.Title)
		b.ReadStatus.MarkAsUnread(*item)
		return nil
	}
}

// SetOfflineMode sets the offline mode of the backend.
func (b *Backend) SetOfflineMode(mode bool) {
	b.Cache.OfflineMode = mode
}

// LoadOPML loads an OPML file.
func (b *Backend) LoadOPML(path string) error {
	return b.Rss.LoadOPML(path)
}

// Close closes the backend and saves its components.
func (b Backend) Close() error {
	if err := b.Rss.Save(); err != nil {
		return err
	}

	if err := b.Cache.Save(); err != nil {
		return err
	}

	return b.ReadStatus.Save()
}

// articlesToSuccessMsg converts a list of items to a FetchArticleSuccessMsg.
func (b Backend) articlesToSuccessMsg(items cache.SortableArticles) FetchArticleSuccessMsg {
	result := make([]list.Item, len(items))
	contents := make([]string, len(items))

	for i, item := range items {
		if b.ReadStatus.IsRead(item) {
			item.Title = "✓ " + item.Title
		}

		result[i] = simplelist.NewItem(item.Title, betterDesc(item.Description))
		contents[i] = rss.YassifyItem(&items[i])
	}

	return FetchArticleSuccessMsg{result, contents}
}

// indexToItem resolves an index to an item.
func (b Backend) indexToItem(feedName string, index int) (*gofeed.Item, error) {
	switch feedName {
	case rss.AllFeedsName:
		return &b.Cache.GetArticlesBulk(rss.Default.GetAllURLs())[index], nil
	case rss.DownloadedFeedsName:
		return &b.Cache.GetDownloaded()[index], nil
	default:
		url, err := b.Rss.GetFeedURL(feedName)
		if err != nil {
			return nil, errors.New("getting the article url")
		}

		items, err := b.Cache.GetArticles(url)
		if err != nil {
			return nil, errors.New("fetching the article")
		}

		return &items[index], nil
	}
}

// betterDesc returns a styled item description.
func betterDesc(rawDesc string) string {
	desc := rawDesc
	text, err := rss.HTMLToText(rawDesc)
	if err == nil {
		desc = text
	}

	return desc
}
