"""Clean dashboard records and received floor markers in one local transaction."""
import hashlib
from core import Store, dumps, utc_now


class DashboardStore(Store):
    @staticmethod
    def marker_snapshot(db):
        tables = {r[0] for r in db.execute("SELECT name FROM sqlite_master WHERE type='table'")}
        zones, suspects = [], []
        if 'floor_j_zones' in tables:
            zones = [dict(r) for r in db.execute("SELECT id,connection_key,data_json FROM floor_j_zones WHERE state='active' ORDER BY id")]
        if 'analysis_v_suspects' in tables:
            suspects = [dict(r) for r in db.execute("SELECT connection_key,robot_id,data_json FROM analysis_v_suspects WHERE state='active' ORDER BY connection_key,robot_id")]
        return {'zones':zones,'suspects':suspects}

    def clean_preview(self, db, through, markers=None):
        base = super()._records_reset_preview(db,through)
        markers = self.marker_snapshot(db) if markers is None else markers
        marker_count = len(markers['zones'])+len(markers['suspects'])
        fingerprint = hashlib.sha256(dumps([base['fingerprint'],markers]).encode()).hexdigest()
        return {**base,'event_count':base['count'],'marker_count':marker_count,
                'count':base['count']+marker_count,'fingerprint':fingerprint,'marker_snapshot':markers}

    def records_reset_preview(self):
        with self.connect() as db:
            db.execute('BEGIN')
            through = db.execute('SELECT COALESCE(MAX(id),0) FROM events').fetchone()[0]
            return self.clean_preview(db,through)

    def reset_records(self, data):
        through, fingerprint, markers = data.get('through_id'),data.get('fingerprint'),data.get('marker_snapshot')
        if data.get('confirm')!='reset_all_records' or type(through) is not int or through<0 or not isinstance(fingerprint,str):
            raise ValueError('Preview and confirm dashboard records before cleaning.')
        if not isinstance(markers,dict) or not all(isinstance(markers.get(k),list) for k in ('zones','suspects')):
            raise ValueError('Reload the page and open Clean records again.')
        for group, columns in (('zones',('id','connection_key','data_json')),('suspects',('connection_key','robot_id','data_json'))):
            if any(not isinstance(row,dict) or not all(isinstance(row.get(k),str) for k in columns) for row in markers[group]):
                raise ValueError('Invalid cleaning snapshot. Reload the page.')
        with self.connect() as db:
            db.execute('BEGIN IMMEDIATE')
            preview = self.clean_preview(db,through,markers)
            if fingerprint != preview['fingerprint']:
                raise ValueError('Records changed. Open Clean records again.')
            stamp = utc_now()
            for row in db.execute('SELECT * FROM events WHERE id<=? ORDER BY id',(through,)):
                db.execute('INSERT INTO cleared_test_events VALUES (?,?,?,?,?)',
                           (row['source'],row['cin_ri'],row['id'],stamp,dumps(dict(row))))
            db.execute('DELETE FROM events WHERE id<=?',(through,))
            zones = suspects = 0
            # Exact snapshot comparisons preserve any newer verdict/zone received while the dialog was open.
            for row in markers['zones']:
                zones += db.execute("UPDATE floor_j_zones SET state='cleaned' WHERE id=? AND connection_key=? AND data_json=? AND state='active'",
                                    (row['id'],row['connection_key'],row['data_json'])).rowcount
            for row in markers['suspects']:
                suspects += db.execute("UPDATE analysis_v_suspects SET state='cleaned' WHERE connection_key=? AND robot_id=? AND data_json=? AND state='active'",
                                       (row['connection_key'],row['robot_id'],row['data_json'])).rowcount
            revision = preview['records_revision']+1
            db.execute("INSERT INTO settings(key,value) VALUES ('test_records_revision',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value",(dumps(revision),))
            return {'cleared':preview['event_count']+zones+suspects,'archived':preview['event_count'],
                    'events_cleared':preview['event_count'],'zones_cleared':zones,'suspects_cleared':suspects,
                    'records_revision':revision,'scope':'all_factories'}
