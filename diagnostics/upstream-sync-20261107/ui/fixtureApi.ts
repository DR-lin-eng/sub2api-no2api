let saved = ['old@gmail.com', 'sales+team@company.example']
export async function getRegistrationEmailBlacklist() { return { emails:saved, max:20000 } }
export async function updateRegistrationEmailBlacklist(emails: string[]) {
 const added = emails.filter(value => !saved.includes(value))
 saved = [...emails]
 return { emails:saved, added, max:20000 }
}
export async function findBlacklistedEmailUsers() { return { items:[{id:23,email:'n.e.w+tag@googlemail.com',username:'fixture'}],total:1 } }
export async function batchDisable(ids: number[]) { return { affected:ids.length } }
export function useAuthStore() { return { isAdmin:true,hasPermission:()=>true } }
export function useAppStore() { return { showSuccess:()=>{},showInfo:()=>{},showError:()=>{} } }
