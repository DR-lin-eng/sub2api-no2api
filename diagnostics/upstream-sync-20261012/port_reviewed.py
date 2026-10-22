#!/usr/bin/env python3
"""Apply reviewed literal hunks to current owners; ambiguous hunks remain pending."""
import json
import pathlib
import re

ROOT = pathlib.Path(__file__).resolve().parents[2]
ROLE = pathlib.Path(__file__).resolve().parent
ALIAS = {
    '@/components/common/Select.vue': '@/common/widgets/forms/Select.vue',
    '@/components/common/GroupBadge.vue': '@/common/widgets/data/GroupBadge.vue',
    '@/components/icons/Icon.vue': '@/common/widgets/icons/Icon.vue',
    '@/components/admin/group/GroupI18nField.vue': '@/features/admin-groups/presentation/widgets/GroupI18nField.vue',
    '@/utils/apiError': '@/core/utils/apiError',
    '@/utils/format': '@/core/utils/format',
    '@/stores/app': '@/core/stores/app',
    '@/i18n': '@/core/i18n',
}

def translate(text):
    for old,new in ALIAS.items(): text=text.replace(old,new)
    for old,new in {
        '/internal/service': '/internal/application/service',
        '/internal/handler': '/internal/transport/http/handler',
        '/internal/repository': '/internal/infrastructure/repository',
        '/internal/config': '/internal/platform/config',
        '/internal/pkg/': '/internal/shared/',
    }.items(): text=text.replace(old,new)
    return text

def owner(path):
    path=translate(path)
    if (ROOT/path).exists(): return ROOT/path
    if path.startswith('frontend/'):
        name=pathlib.Path(path).name.replace('Modal.vue','Dialog.vue').replace('View.vue','Page.vue')
        if name=='ChannelStatusV1Page.vue': name='ChannelStatusPage.vue'
        matches=list((ROOT/'frontend/src').rglob(name))
        if len(matches)==1:return matches[0]
    return ROOT/path

log=[]
for n in [7982,7983,7974,7973,7972,7970,7969,7717,7696,8002,7649,7711,7960]:
    for section in (ROLE/f'PR-{n}.diff').read_text().split('diff --git ')[1:]:
        oldpath=section.splitlines()[0].split(' ')[0][2:]
        # Generated Ent code is regenerated from schema. Split forms and DTOs are adapted separately.
        if ('_test.go' in oldpath or '.spec.ts' in oldpath or
            oldpath.startswith('backend/migrations/') or
            oldpath.startswith('backend/ent/') and not oldpath.startswith('backend/ent/schema/') or
            oldpath in ['frontend/src/types/index.ts','frontend/src/views/admin/GroupsView.vue']):continue
        path=owner(oldpath)
        if 'new file mode' in section:
            body=''.join(line[1:]+'\n' for line in section.splitlines() if line.startswith('+') and not line.startswith('+++'))
            if path.exists(): raise RuntimeError('Unexpected collision: '+str(path))
            path.parent.mkdir(parents=True,exist_ok=True);path.write_text(translate(body))
            log.append({'pr':n,'path':str(path.relative_to(ROOT)),'state':'added'});continue
        chunks=re.split(r'^@@[^\n]*\n',section,flags=re.M)[1:]
        for index,chunk in enumerate(chunks):
            before='';after=''
            for line in chunk.splitlines(keepends=True):
                if line.startswith(' '):before+=line[1:];after+=line[1:]
                elif line.startswith('-'):before+=line[1:]
                elif line.startswith('+'):after+=line[1:]
            before=translate(before);after=translate(after)
            candidates=[path] if path.exists() else []
            if oldpath.startswith('backend/internal/'):
                candidates=list(dict.fromkeys(candidates+list(path.parent.glob('*.go'))))
            hits=[p for p in candidates if p.is_file() and p.read_text().count(before)==1]
            if len(hits)!=1:
                log.append({'pr':n,'path':str(path.relative_to(ROOT)),'hunk':index,'state':'pending','before':before,'after':after})
                continue
            found=hits[0];found.write_text(found.read_text().replace(before,after,1))
            log.append({'pr':n,'path':str(found.relative_to(ROOT)),'hunk':index,'state':'applied'})
(ROLE/'PORT_HUNKS.json').write_text(json.dumps(log,ensure_ascii=False,indent=2)+'\n')
print('Applied:',sum(x['state']!='pending' for x in log),'Pending:',sum(x['state']=='pending' for x in log))
for x in log:
    if x['state']=='pending':print(x['pr'],x['path'],x['hunk'])
