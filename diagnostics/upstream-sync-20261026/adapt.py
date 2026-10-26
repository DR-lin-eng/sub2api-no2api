#!/usr/bin/env python3
"""Apply frozen #8323 behavior to the fork's current owners; record pre-write hashes."""
from pathlib import Path
import hashlib,json,re,subprocess
R=Path(__file__).resolve().parents[2];D=Path(__file__).resolve().parent
hashes={}
def put(p,s):
 f=R/p
 if f.exists():
  old=f.read_bytes()
  if old==s.encode():return
  hashes.setdefault(p,hashlib.sha256(old).hexdigest())
 else:hashes.setdefault(p,None)
 f.parent.mkdir(parents=True,exist_ok=True);f.write_text(s)
def edit(p,a,b,n=1):
 s=(R/p).read_text();assert s.count(a)==n,(p,a,s.count(a),n);put(p,s.replace(a,b))
paths=['backend/internal/application/service/'+p for p in ['group.go','admin_service.go','admin_group.go','admin_group_duplicate.go','api_key_auth_cache.go','api_key_auth_cache_impl.go']]+['backend/internal/infrastructure/repository/'+p for p in ['group_repo.go','api_key_repo.go']]+['backend/internal/transport/http/handler/'+p for p in ['admin/group_handler.go','dto/types.go','dto/mappers.go']]
for p in paths:
 s=(R/p).read_text();out=[]
 for line in s.splitlines(True):
  out.append(line)
  if 'AllowBatchImageGeneration' in line and (':' in line or re.search(r'AllowBatchImageGeneration\s+\*?bool',line) or 'SetAllowBatchImageGeneration(' in line or 'group.FieldAllowBatchImageGeneration,' in line):
   extra=line.replace('AllowBatchImageGeneration','ImageCountBilling').replace('allow_batch_image_generation','image_count_billing').replace('allowBatchImageGeneration','imageCountBilling')
   out.append(extra)
 put(p,''.join(out))
edit('backend/internal/application/service/api_key_auth_cache_impl.go','const apiKeyAuthSnapshotVersion = 25 // v25: include group distillation policy','const apiKeyAuthSnapshotVersion = 26 // v26: include group image_count_billing; preserve v25 distillation policy')
p='backend/internal/application/service/admin_group.go'
a='\tallowBatchImageGeneration := input.AllowBatchImageGeneration && allowImageGeneration && platform == PlatformGemini\n'
edit(p,a,a+'\timageCountBilling := input.ImageCountBilling && allowImageGeneration &&\n\t\t(platform == PlatformComposite || platform == PlatformOpenAI)\n')
a='\tif !group.AllowImageGeneration || group.Platform != PlatformGemini {\n\t\tgroup.AllowBatchImageGeneration = false\n\t}\n'
edit(p,a,a+'\tif input.ImageCountBilling != nil {\n\t\tgroup.ImageCountBilling = *input.ImageCountBilling\n\t}\n\tif !group.AllowImageGeneration || (group.Platform != PlatformComposite && group.Platform != PlatformOpenAI) {\n\t\tgroup.ImageCountBilling = false\n\t}\n')
p='backend/internal/application/service/group_service.go';s=(R/p).read_text();out=[]
for line in s.splitlines(True):
 out.append(line)
 if re.search(r'^\s*AllowImageGeneration\s+\*?bool',line) or 'AllowImageGeneration: req.AllowImageGeneration,' in line:
  out.append(line.replace('AllowImageGeneration','ImageCountBilling').replace('allow_image_generation','image_count_billing'))
put(p,''.join(out))
a='func (s *GroupService) Create(ctx context.Context, req CreateGroupRequest) (*Group, error) {\n'
edit(p,a,a+'\tif req.ImageCountBilling {\n\t\treturn nil, fmt.Errorf("image_count_billing requires an OpenAI or composite group with image generation enabled")\n\t}\n')
a='\tif req.AllowImageGeneration != nil {\n\t\tgroup.AllowImageGeneration = *req.AllowImageGeneration\n\t}\n'
edit(p,a,a+'\tif req.ImageCountBilling != nil {\n\t\tif *req.ImageCountBilling && (!group.AllowImageGeneration || (group.Platform != PlatformComposite && group.Platform != PlatformOpenAI)) {\n\t\t\treturn nil, fmt.Errorf("image_count_billing requires an OpenAI or composite group with image generation enabled")\n\t\t}\n\t\tgroup.ImageCountBilling = *req.ImageCountBilling\n\t}\n\tif !group.AllowImageGeneration || (group.Platform != PlatformComposite && group.Platform != PlatformOpenAI) {\n\t\tgroup.ImageCountBilling = false\n\t}\n')
p='backend/ent/schema/group.go';a='\t\tfield.Bool("image_rate_independent").';edit(p,a,'\t\tfield.Bool("image_count_billing").\n\t\t\tDefault(false).\n\t\t\tComment("专用图片端点是否按实际出图张数计费"),\n'+a)
for p in ['gateway_service.go','openai_gateway_service.go']:
 p='backend/internal/application/service/'+p;s=(R/p).read_text();s,n=re.subn(r'(?m)^(\s*ImageCount\s+int[^\n]*\n)',r'\1\t// ImageEndpoint is set only by dedicated image endpoints, never by mixed text/image output.\n\tImageEndpoint bool\n',s);assert n==1;put(p,s)
p='backend/internal/application/service/gateway_usage_billing.go'
a='if resolved != nil && isExplicitAdminPricing(resolved) && resolved.Mode == BillingModeToken {'
edit(p,a,'if !imageCountBillingApplies(apiKey, result.ImageEndpoint) && resolved != nil && isExplicitAdminPricing(resolved) && resolved.Mode == BillingModeToken {')
a='if resolved != nil && isExplicitAdminPricing(resolved) && s.resolver != nil && apiKey != nil && apiKey.Group != nil {'
edit(p,a,'if resolved != nil && !(imageCountBillingApplies(apiKey, result.ImageEndpoint) && resolved.Mode == BillingModeToken) && isExplicitAdminPricing(resolved) && s.resolver != nil && apiKey != nil && apiKey.Group != nil {')
p='backend/internal/application/service/openai_gateway_usage.go'
a='if resolved := s.resolveOpenAIChannelPricing(ctx, billingModel, apiKey); resolved == nil || resolved.Mode != BillingModeToken {'
s=(R/p).read_text();at=s.index('\tif result != nil && result.ImageCount > 0 {',s.index('func (s *OpenAIGatewayService) calculateOpenAIRecordUsageCostWithResolution'))
before,after=s[:at],s[at:];assert a in after;after=after.replace(a,'if resolved := s.resolveOpenAIChannelPricing(ctx, billingModel, apiKey); imageCountBillingApplies(apiKey, result.ImageEndpoint) || resolved == nil || resolved.Mode != BillingModeToken {',1);put(p,before+after)
# Per-request context tiers need token counts even though the final cost is per image.
s=(R/p).read_text();start=s.index('func (s *OpenAIGatewayService) calculateOpenAIImageCost(');end=s.index('\nfunc ',start+5);a=s[start:end];a=a.replace('RequestCount: result.ImageCount, SizeTier: sizeTier,','Tokens: UsageTokens{InputTokens: result.Usage.InputTokens, OutputTokens: result.Usage.OutputTokens, ImageOutputTokens: result.Usage.ImageOutputTokens},\n\t\t\tRequestCount: result.ImageCount, SizeTier: sizeTier,');a=a.replace('\t\t\tRequestCount:   result.ImageCount,','\t\t\tTokens: UsageTokens{InputTokens: result.Usage.InputTokens, OutputTokens: result.Usage.OutputTokens, ImageOutputTokens: result.Usage.ImageOutputTokens},\n\t\t\tRequestCount:   result.ImageCount,');put(p,s[:start]+a+s[end:])
helper=subprocess.check_output(['git','show','upstream/main:backend/internal/service/image_billing_count.go'],cwd=R,text=True)
put('backend/internal/application/service/image_billing_count.go',helper)
helper='''package service

// imageCountBillingEnabled is opt-in; missing fields from old nodes remain false.
func imageCountBillingEnabled(apiKey *APIKey) bool {
 return apiKey != nil && apiKey.Group != nil && apiKey.Group.ImageCountBilling
}

// Mixed Responses, Chat and WS output must retain its text token charges.
func imageCountBillingApplies(apiKey *APIKey, imageEndpoint bool) bool {
 return imageEndpoint && imageCountBillingEnabled(apiKey)
}

'''
p='backend/internal/application/service/media_price_config.go';put(p,helper+(R/p).read_text().removeprefix('package service\n\n'))
for p in ['openai_images.go','openai_images_responses.go']:
 p='backend/internal/application/service/'+p;s=(R/p).read_text()
 s=re.sub(r'(?m)^(\s*)ImageCount:(\s+)',r'\1ImageEndpoint: true,\n\1ImageCount:\2',s)
 if p.endswith('/openai_images.go'):
  s=s.replace('imageCount := parsed.N','var imageCount int').replace('imageCount = streamCount','imageCount = resolveBillableImageCount(streamCount, 0, imageCountBillingEnabled(getAPIKeyFromContext(c)))')
  a='\t\tif nonStreamCount > 0 {\n\t\t\timageCount = nonStreamCount\n\t\t}'
  assert a in s;s=s.replace(a,'\t\timageCount = resolveBillableImageCount(nonStreamCount, parsed.N, imageCountBillingEnabled(getAPIKeyFromContext(c)))')
 else:
  a='\tif imageCount <= 0 {\n\t\timageCount = parsed.N\n\t}'
  assert a in s;s=s.replace(a,'\timageCount = resolveBillableImageCount(imageCount, parsed.N, imageCountBillingEnabled(getAPIKeyFromContext(c)))')
 put(p,s)
# Use the fork's next migration number, never upstream's occupied 248.
nums=[int(p.name.split('_')[0]) for p in (R/'backend/migrations').glob('[0-9]*_*.sql') if p.name.split('_')[0].isdigit()];number=max(nums)+1
put(f'backend/migrations/{number:03d}_group_image_count_billing.sql','-- Opt-in per-image billing; false preserves existing installations.\nALTER TABLE groups ADD COLUMN IF NOT EXISTS image_count_billing BOOLEAN NOT NULL DEFAULT false;\nCOMMENT ON COLUMN groups.image_count_billing IS \'专用图片端点是否按实际出图张数计费\';\n')
# Feature-owned form/DTO adaptation instead of copying upstream's monolithic view.
p='frontend/src/types/group.ts';edit(p,'  allow_batch_image_generation: boolean\n','  allow_batch_image_generation: boolean\n  image_count_billing?: boolean\n')
for p in (R/'frontend/src').rglob('*.ts'):
 if 'types' not in p.parts and 'dtos' not in p.parts:continue
 s=p.read_text()
 if 'allow_batch_image_generation?: boolean' in s:put(str(p.relative_to(R)),s.replace('allow_batch_image_generation?: boolean','allow_batch_image_generation?: boolean\n  image_count_billing?: boolean'))
for p in ['groupEditorFormSupport.ts','useCreateGroupController.ts','useEditGroupController.ts']:
 p='frontend/src/features/admin-groups/presentation/composables/'+p;s=(R/p).read_text()
 s=s.replace('    allow_batch_image_generation: false,','    allow_batch_image_generation: false,\n    image_count_billing: false,')
 s=s.replace('    createForm.allow_batch_image_generation = false;','    createForm.allow_batch_image_generation = false;\n    createForm.image_count_billing = false;')
 s=s.replace('    editForm.image_rate_independent = group.image_rate_independent ?? false;','    editForm.image_count_billing = group.image_count_billing ?? false;\n    editForm.image_rate_independent = group.image_rate_independent ?? false;')
 s=s.replace('  allow_batch_image_generation: boolean;','  allow_batch_image_generation: boolean;\n  image_count_billing: boolean;').replace('    | "allow_batch_image_generation"','    | "allow_batch_image_generation"\n    | "image_count_billing"')
 if p.endswith('groupEditorFormSupport.ts'):
  a='  if (form.platform !== "gemini" || !form.allow_image_generation) {'
  s=s.replace(a,'  if (!form.allow_image_generation || !["openai", "composite"].includes(form.platform)) {\n    form.image_count_billing = false;\n  }\n'+a)
 put(p,s)
p='frontend/src/features/admin-groups/presentation/widgets/GroupEditorMediaPricingFields.vue';s=(R/p).read_text();a='      <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">\n        <input\n          v-model="form.image_rate_independent"'
b='''      <label
        v-if="form.allow_image_generation && ['openai', 'composite'].includes(form.platform)"
        class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300"
        :title="t('admin.groups.imagePricing.perImageBillingHint')"
      >
        <input v-model="form.image_count_billing" type="checkbox" class="rounded border-gray-300 text-blue-600 focus:ring-blue-500" />
        {{ t("admin.groups.imagePricing.perImageBilling") }}
      </label>
'''+a
assert a in s;put(p,s.replace(a,b))
for lang,label,hint in [('en','Bill per generated image','Dedicated image endpoints only. Enable after all instances are upgraded; mixed text/image requests keep token billing.'),('zh','按实际出图张数计费','仅对专用图片端点生效。全部实例升级后再开启；混合文本与图片请求仍按 token 计费。')]:
 p=f'frontend/src/core/i18n/locales/{lang}/admin/overview.ts';s=(R/p).read_text();a='        allowBatchImageGeneration:';at=s.index(a);s=s[:at]+f"        perImageBilling: '{label}',\n        perImageBillingHint: '{hint}',\n"+s[at:];put(p,s)
(D/'PREWRITE_HASHES.json').write_text(json.dumps(hashes,indent=2)+'\n')
print(json.dumps({'changed_files':len(hashes),'migration':number,'prewrite_hashes':str(D/'PREWRITE_HASHES.json')}))
