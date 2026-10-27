"""Real HTTP key buckets across upgrade/rollback; secrets stay in process memory."""
from common import *
report=[]
try:
    compose('up','-d','postgres','redis');start(BASELINE)
    token,uid=login()
    ack=json.dumps({'version':'v2026.06.10','admin_user_id':uid,'accepted_at':'2026-10-26T00:00:00Z'})
    sql(f"INSERT INTO settings(key,value) VALUES ('admin_compliance_acknowledgement:{uid}','{ack}')")
    keys=[new_admin_key(token,'fixture-key-'+str(i)) for i in range(3)]
    code,_=request('/api/v1/admin/settings/panel-rate-limit',{'enabled':True,'user_rpm':1,'heavy_rpm':1,'exempt_admin':True,'public_ip_rpm':300},token,method='PUT')
    assert code==200,('set panel policy',code)
    for stage,image,fixed in [('BASELINE',BASELINE,False),('MODIFIED',CANDIDATE,True),('ROLLBACK',BASELINE,False),('RESTORED',CANDIDATE,True)]:
        start(image,True);compose('exec','-T','redis','redis-cli','FLUSHDB')
        token,current=login();assert current==uid
        codes=[admin_key_request('/api/v1/admin/users',keys[i]) for i in [0,1,0]]
        assert codes==([200,200,429] if fixed else [200,200,200]),(stage,codes)
        assert request('/api/v1/admin/users',token=token)[0]==200
        assert request('/api/v1/admin/users',token=token)[0]==200
        # Even a machine key with both broad scopes cannot manage credentials.
        assert admin_key_request('/api/v1/admin/settings/admin-api-keys',keys[2],{'name':'forbidden','scopes':['admin.read']})==403
        item={'stage':stage,'result':'PASS','key_a_key_b_key_a_http':codes,'jwt_admin_exemption':'PASS','machine_key_lifecycle_denied':403}
        report.append(item);print(json.dumps(item),flush=True)
    (ROOT/'key-result.json').write_text(json.dumps(report,indent=2)+'\n')
finally:compose('down','--volumes','--remove-orphans')
