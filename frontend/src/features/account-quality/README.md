# Public account quality

`presentation/pages/AccountQualitySharePage.vue` is the anonymous, aggregate quality dashboard at `/monitor/quality/public`. `data/datasources/accountQualityPublicDatasource.ts` owns the read-only `/account-quality-share` contract. It shows the normal/unnormal timeline, classifier confidence, model version, generated WebP previews, and rolling counts. Account IDs, prompts, raw HTML, and credentials are never rendered here.
