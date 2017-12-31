package indexer

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
)

type Indexer struct {
	GitHubUrl     string
	Content       string
	Renderer      *Renderer
	SyntaxChecker *SyntaxChecker
	ctx           context.Context
}

type Uml struct {
	GitHubUrl   string      `datastore:"gitHubUrl"`
	Source      string      `datastore:"source,noindex"`
	DiagramType DiagramType `datastore:"diagramType"`
	Svg         string      `datastore:"svg,noindex"`
	PngBase64   string      `datastore:"pngBase64,noindex"`
	Ascii       string      `datastore:"ascii,noindex"`
}

type DiagramType string

const (
	TypeSequence  DiagramType = "sequence"
	TypeUsecase   DiagramType = "usecase"
	TypeClass     DiagramType = "class"
	TypeActivity  DiagramType = "activity"
	TypeComponent DiagramType = "component"
	TypeState     DiagramType = "state"
	TypeObject    DiagramType = "object"
	TypeUnknwon   DiagramType = "__unknown__"
)

func guessDiagramType(source string, result *SyntaxCheckResult) DiagramType {
	switch result.DiagramType {
	case "SEQUENCE":
		return TypeSequence
	case "DESCRIPTION":
		// Both of Usecase and Component diagram's syntax check result are "DESCRIPTION",
		// so distinct them ad hoc
		if strings.Contains(source, "actor") || strings.Contains(source, "usecase") {
			return TypeUsecase
		} else {
			return TypeComponent
		}
	case "CLASS":
		return TypeClass
	case "ACTIVITY":
		return TypeActivity
	case "STATE":
		return TypeState
	case "Object":
		return TypeObject
	default:
		return TypeUnknwon
	}
}

func NewIndexer(ctx context.Context, renderer *Renderer, syntaxChecker *SyntaxChecker, gitHubUrl string, content string) (*Indexer, error) {
	return &Indexer{
		Content:       content,
		Renderer:      renderer,
		SyntaxChecker: syntaxChecker,
		GitHubUrl:     gitHubUrl,
		ctx:           ctx,
	}, nil
}

func (idxr *Indexer) FindSources() []string {
	sources := make([]string, 0)
	content := idxr.Content
	for {
		startIdx := strings.Index(content, "@startuml")
		endIdx := strings.Index(content, "@enduml")
		if startIdx == -1 || endIdx == -1 {
			break
		}

		// TODO: 最小文字数チェック
		source := fmt.Sprintf("%s@enduml", content[startIdx:endIdx])
		sources = append(sources, source)

		content = content[(endIdx + len("@enduml")):]
	}
	return sources
}

func (idxr *Indexer) Process() error {
	ctx := idxr.ctx
	sources := idxr.FindSources()
	renderer := idxr.Renderer
	syntaxChecker := idxr.SyntaxChecker

	for _, source := range sources {
		log.Infof(ctx, "process source: %s", source)

		result, err := syntaxChecker.CheckSyntax(source)
		if err != nil {
			log.Criticalf(ctx, "failed to check syntax: %s", err)
			return err
		}
		log.Infof(ctx, "syntax check result: %v", result)

		if !result.Valid {
			log.Infof(ctx, "invalid syntax: %s", source)
			continue
		}

		typ := guessDiagramType(source, result)

		svg, err := renderer.RenderSvg(source)
		if err != nil {
			log.Criticalf(ctx, "failed to render svg: %s", err)
			return err
		}

		png, err := renderer.RenderPng(source)
		if err != nil {
			log.Criticalf(ctx, "failed to render png: %s", err)
			return err
		}
		pngBase64 := base64.StdEncoding.EncodeToString(png)

		ascii, err := renderer.RenderAscii(source)
		if err != nil {
			log.Criticalf(ctx, "failed to render ascii: %s", err)
			return err
		}

		log.Infof(ctx, "make index: type=%s, svg=%s, pngBase64=%s, ascii=%s", typ, svg, pngBase64, ascii)
		uml := &Uml{
			GitHubUrl:   idxr.GitHubUrl,
			Source:      source,
			DiagramType: typ,
			Svg:         svg,
			PngBase64:   pngBase64,
			Ascii:       ascii,
		}

		key := datastore.NewIncompleteKey(ctx, "Uml", nil)
		key, err = datastore.Put(ctx, key, uml)
		if err != nil {
			log.Criticalf(ctx, "put error: %s", err)
			return err
		}
	}

	return nil
}
