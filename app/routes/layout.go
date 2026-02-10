package routes

import (
	"github.com/vango-go/vango"
	. "github.com/vango-go/vango/el"
)

func Layout(ctx vango.Ctx, children vango.Slot) *vango.VNode {
	return Html(
		Lang("en"),
		Head(
			Meta(Charset("utf-8")),
			Meta(Name("viewport"), Content("width=device-width, initial-scale=1")),
			Title(Text("rhone_chat")),
			LinkEl(Rel("stylesheet"), Href(ctx.Asset("styles.css"))),
		),
		Body(Class("h-screen overflow-hidden"),
			children,
			VangoScripts(),
		),
	)
}
