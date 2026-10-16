-- Synthetic browser fixtures, applied only inside the runner's disposable schema.
DO $$
DECLARE
  school uuid; year_id uuid; class_a uuid; class_b uuid; owner_id uuid;
  role_id uuid; user_id uuid; student_id uuid; enrollment_id uuid; schedule_id uuid; session_id uuid;
  password text; item record; class_id uuid; day date;
BEGIN
  SELECT id INTO school FROM schools;
  SELECT id,password_hash INTO owner_id,password FROM users WHERE email='browser-admin@example.test';
  day := (clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date;
  INSERT INTO academic_years(school_id,name,starts_on,ends_on,timezone,is_active)
    VALUES(school,'Browser year',day-30,day+365,'Asia/Jakarta',true) RETURNING id INTO year_id;
  INSERT INTO classes(school_id,academic_year_id,code,name) VALUES(school,year_id,'A','Browser A') RETURNING id INTO class_a;
  INSERT INTO classes(school_id,academic_year_id,code,name) VALUES(school,year_id,'B','Browser B') RETURNING id INTO class_b;
  FOR item IN SELECT * FROM (VALUES
    ('students','Browser students',ARRAY['students.write']::text[]),
    ('reports','Browser reports',ARRAY['reports.export']::text[]),
    ('approver','Browser approver',ARRAY['attendance.approve']::text[])
  ) AS fixtures(key,name,permissions) LOOP
    INSERT INTO roles(school_id,key,name,scope) VALUES(school,item.key,item.name,'assigned_classes') RETURNING id INTO role_id;
    INSERT INTO role_permissions(school_id,role_id,permission) SELECT school,role_id,unnest(item.permissions);
    INSERT INTO users(school_id,email,full_name,password_hash,role)
      VALUES(school,item.key||'@example.test',item.name,password,item.key) RETURNING id INTO user_id;
    INSERT INTO user_class_assignments(school_id,user_id,class_id,academic_year_id,starts_at,ends_at)
      VALUES(school,user_id,class_a,year_id,(day-1)::timestamp AT TIME ZONE 'Asia/Jakarta',(day+365)::timestamp AT TIME ZONE 'Asia/Jakarta');
  END LOOP;
  FOR item IN SELECT * FROM (VALUES ('001','Browser Student A',class_a),('002','Browser Student B',class_b)) AS fixtures(nis,name,assigned_class) LOOP
    class_id := item.assigned_class;
    INSERT INTO students(school_id,nis,full_name) VALUES(school,item.nis,item.name) RETURNING id INTO student_id;
    INSERT INTO student_enrollments(school_id,student_id,class_id,academic_year_id,joined_at,left_at,reason)
      VALUES(school,student_id,class_id,year_id,(day-1)::timestamp AT TIME ZONE 'Asia/Jakarta',(day+365)::timestamp AT TIME ZONE 'Asia/Jakarta','Browser fixture') RETURNING id INTO enrollment_id;
    INSERT INTO attendance_schedules(school_id,class_id,name,timezone,weekdays,checkin_start,checkin_cutoff,close_time)
      VALUES(school,class_id,'Browser session','Asia/Jakarta',ARRAY[1,2,3,4,5,6,7],'00:00','00:01','23:59') RETURNING id INTO schedule_id;
    INSERT INTO attendance_sessions(school_id,schedule_id,class_id,date,name,timezone,starts_at,late_after,closes_at,status,opened_at)
      VALUES(school,schedule_id,class_id,day,'Browser session','Asia/Jakarta',day::timestamp AT TIME ZONE 'Asia/Jakarta',(day+'00:01'::time) AT TIME ZONE 'Asia/Jakarta',(day+'23:59'::time) AT TIME ZONE 'Asia/Jakarta','open',clock_timestamp()) RETURNING id INTO session_id;
    INSERT INTO attendance_session_participants(school_id,session_id,student_id,class_id,enrollment_id,nis,full_name)
      VALUES(school,session_id,student_id,class_id,enrollment_id,item.nis,item.name);
    INSERT INTO attendance_records(school_id,session_id,student_id,status,check_in_at,reason,actor_id)
      VALUES(school,session_id,student_id,'present',day::timestamp AT TIME ZONE 'Asia/Jakarta','Browser fixture',owner_id);
    INSERT INTO attendance_corrections(school_id,session_id,student_id,requested_by,previous_version,previous_status,new_status,reason)
      VALUES(school,session_id,student_id,owner_id,1,'present','sick','Browser correction');
    IF item.nis='001' THEN
      INSERT INTO students(school_id,nis,full_name) VALUES(school,'004','Browser Attendance Student') RETURNING id INTO student_id;
      INSERT INTO student_enrollments(school_id,student_id,class_id,academic_year_id,joined_at,left_at,reason)
        VALUES(school,student_id,class_id,year_id,(day-1)::timestamp AT TIME ZONE 'Asia/Jakarta',(day+365)::timestamp AT TIME ZONE 'Asia/Jakarta','Browser attendance fixture') RETURNING id INTO enrollment_id;
      INSERT INTO attendance_session_participants(school_id,session_id,student_id,class_id,enrollment_id,nis,full_name)
        VALUES(school,session_id,student_id,class_id,enrollment_id,'004','Browser Attendance Student');
    END IF;
  END LOOP;
END $$;

-- Public holiday fixtures stay isolated and prevent external provider calls in browser tests.
INSERT INTO school_holiday_sync(school_id,year,synced_at,status)
SELECT s.id,y,clock_timestamp(),'ready' FROM schools s
CROSS JOIN generate_series(extract(year FROM clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::integer,extract(year FROM clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::integer+1) y;
INSERT INTO school_holidays(school_id,date,name,type)
SELECT id,(date_trunc('month',clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date+24),'Libur nasional uji','national_holiday' FROM schools
UNION ALL
SELECT id,(date_trunc('month',clock_timestamp() AT TIME ZONE 'Asia/Jakarta')::date+25),'Cuti bersama uji','joint_leave' FROM schools;
