import {createApp,defineComponent,h,ref} from 'vue'
import {createPinia} from 'pinia'
import {createI18n} from 'vue-i18n'
import zh from '../src/core/i18n/locales/zh'
import en from '../src/core/i18n/locales/en'
import AccountUsageCell from '../src/features/admin-accounts/presentation/widgets/AccountUsageCell.vue'
import UseKeyDialog from '../src/features/keys/presentation/widgets/UseKeyDialog.vue'
import '../src/core/themes/style.css'
const i18n=createI18n({legacy:false,locale:'zh',messages:{zh,en}})
const account={id:99001,name:'Fixture',platform:'antigravity',type:'oauth',extra:{},credentials:{},status:'active',schedulable:true}
const app=createApp(defineComponent({setup(){const show=ref(false);return()=>h('main',{style:'padding:24px;max-width:600px;margin:auto'},[h('button',{onClick:()=>{i18n.global.locale.value=i18n.global.locale.value==='zh'?'en':'zh'}},'中文 / English'),h('div',{style:'margin:28px 0'},[h(AccountUsageCell,{account})]),h('button',{onClick:()=>show.value=true},'Codex 配置 / configuration'),h(UseKeyDialog,{show:show.value,platform:'openai',baseUrl:'https://fixture.invalid',apiKey:'sk-fixture-placeholder',onClose:()=>show.value=false})])}}))
app.use(createPinia()).use(i18n).mount('#app')
