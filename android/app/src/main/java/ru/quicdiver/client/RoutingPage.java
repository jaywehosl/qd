package ru.quicdiver.client;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.Intent;
import android.content.pm.ApplicationInfo;
import android.content.pm.PackageInfo;
import android.content.pm.PackageManager;
import android.content.pm.ResolveInfo;
import android.graphics.Typeface;
import android.graphics.drawable.Drawable;
import android.graphics.drawable.GradientDrawable;
import android.net.Uri;
import android.text.Editable;
import android.text.TextWatcher;
import android.util.TypedValue;
import android.view.Gravity;
import android.view.View;
import android.widget.EditText;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.nio.charset.StandardCharsets;
import java.text.SimpleDateFormat;
import java.util.ArrayList;
import java.util.Collections;
import java.util.Date;
import java.util.HashMap;
import java.util.HashSet;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public class RoutingPage {

    public static final int SAVE_RULES = 41;
    public static final int LOAD_RULES = 42;

    private static final String[] ROLES = {"direct", "noEgress", "egress", "tunnel"};
    private static final String[] NAMES = {"напрямую", "−egress", "+egress", "туннель"};

    private static final String[] BASE_ROLES = {"direct", "tunnel"};
    private static final String[] BASE_NAMES = {"напрямую", "туннель"};

    private final Activity host;
    private final Skin skin;
    private final Map<String, String> labels = new HashMap<>();

    private LinearLayout defaultRow;
    private LinearLayout list;

    private String defaultRole = "tunnel";
    private final List<Rule> rules = new ArrayList<>();
    private List<App> catalogue;
    private final Map<String, Drawable> faces = new HashMap<>();
    private ExecutorService loaders;
    private boolean loaded;
    private volatile boolean loading;
    private String shownRole = "";

    private static class Rule {
        int id;
        String pkg;
        String role;
    }

    private static class App {
        String pkg;
        String label;
        boolean system;
    }

    public RoutingPage(Activity host, Skin skin) {
        this.host = host;
        this.skin = skin;
    }

    public View build() {
        LinearLayout page = skin.column();
        page.setPadding(skin.dp(24), 0, skin.dp(24), 0);

        LinearLayout everything = skin.card();
        everything.addView(skin.title("Захват всего трафика по умолчанию"));
        everything.addView(skin.note(
                "Как захватывается и маршрутизируется трафик приложений без своего правила "
                        + "и трафик самой системы"));

        defaultRow = skin.column();
        LinearLayout.LayoutParams rowAt = new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT);
        rowAt.topMargin = skin.dp(12);
        everything.addView(defaultRow, rowAt);
        page.addView(everything, skin.gap(14));

        LinearLayout box = skin.card();
        LinearLayout bar = new LinearLayout(host);
        bar.setOrientation(LinearLayout.HORIZONTAL);
        bar.setGravity(Gravity.CENTER_VERTICAL);
        bar.addView(skin.title("Правила"),
                new LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f));

        TextView add = skin.button("Выбрать приложение", skin.good);
        add.setOnClickListener(new View.OnClickListener() {
            @Override
            public void onClick(View v) {
                pickApp();
            }
        });
        bar.addView(add);
        box.addView(bar);

        list = skin.column();
        box.addView(list);

        LinearLayout files = skin.segments(new String[]{"Сохранить в файл", "Загрузить из файла"}, -1,
                new Skin.Pick() {
                    @Override
                    public void at(int index) {
                        Intent pick;
                        if (index == 0) {
                            pick = new Intent(Intent.ACTION_CREATE_DOCUMENT);
                            pick.setType("application/octet-stream");
                            pick.putExtra(Intent.EXTRA_TITLE, "qd-routing-android-"
                                    + new SimpleDateFormat("yyyy-MM-dd_HH-mm-ss", Locale.US).format(new Date()) + ".qdr");
                        } else {
                            pick = new Intent(Intent.ACTION_OPEN_DOCUMENT);
                            pick.setType("*/*");
                        }
                        pick.addCategory(Intent.CATEGORY_OPENABLE);
                        host.startActivityForResult(pick, index == 0 ? SAVE_RULES : LOAD_RULES);
                    }
                });
        LinearLayout.LayoutParams filesAt = new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT);
        filesAt.topMargin = skin.dp(12);
        box.addView(files, filesAt);
        page.addView(box, skin.gap(14));

        Scroller scroll = new Scroller(host, skin);
        scroll.setClipChildren(false);
        scroll.setClipToPadding(false);
        page.setClipChildren(false);
        page.setClipToPadding(false);
        scroll.addView(page);
        scroll.setFillViewport(true);
        return scroll;
    }

    public void render() {
        if (!loaded) {
            load();
        }
        if (!defaultRole.equals(shownRole)) {
            shownRole = defaultRole;
            defaultRow.removeAllViews();
            defaultRow.addView(skin.segments(BASE_NAMES, indexIn(BASE_ROLES, defaultRole),
                    new Skin.Pick() {
                        @Override
                        public void at(int index) {
                            defaultRole = BASE_ROLES[index];
                            render();
                            save();
                        }
                    }));
        }
    }

    private void load() {
        if (loading) {
            return;
        }
        loading = true;

        new Thread(new Runnable() {
            @Override
            public void run() {
                String raw = null;
                String problem = null;
                try {
                    raw = Core.client(host).rulesJSON();
                } catch (Exception e) {
                    problem = String.valueOf(e.getMessage());
                }

                final String body = raw;
                final String said = problem;
                host.runOnUiThread(new Runnable() {
                    @Override
                    public void run() {
                        loading = false;
                        if (said != null) {
                            say(said, true);
                            return;
                        }
                        take(body);
                    }
                });
            }
        }).start();
    }

    private void take(String raw) {
        try {
            JSONObject body = new JSONObject(raw);
            String kept = body.optString("defaultRole", "tunnel");
            boolean stray = !"direct".equals(kept) && !"tunnel".equals(kept);
            defaultRole = stray ? "tunnel" : kept;

            rules.clear();
            JSONArray rows = body.optJSONArray("rules");
            for (int i = 0; rows != null && i < rows.length(); i++) {
                JSONObject row = rows.getJSONObject(i);
                Rule rule = new Rule();
                rule.id = row.optInt("id");
                rule.pkg = row.optString("process");
                rule.role = row.optString("role", "tunnel");
                rules.add(rule);
            }
            loaded = true;
            redraw();
            render();
            if (stray) {
                save();
            }
        } catch (Exception e) {
            say(String.valueOf(e.getMessage()), true);
        }
    }

    private void redraw() {
        list.removeAllViews();
        if (rules.isEmpty()) {
            TextView blank = skin.label("здесь будут ваши правила маршрутизации", skin.muted, 14);
            blank.setGravity(Gravity.CENTER);
            blank.setPadding(0, skin.dp(26), 0, skin.dp(10));
            list.addView(blank);
            return;
        }
        for (Rule rule : rules) {
            list.addView(row(rule));
        }
    }

    private View row(final Rule rule) {
        LinearLayout block = skin.column();
        block.setPadding(0, skin.dp(14), 0, skin.dp(2));

        LinearLayout line = new LinearLayout(host);
        line.setOrientation(LinearLayout.HORIZONTAL);
        line.setGravity(Gravity.CENTER_VERTICAL);

        ImageView face = new ImageView(host);
        GradientDrawable blank = new GradientDrawable();
        blank.setColor(skin.idle);
        blank.setCornerRadius(skin.dp(8));
        face.setBackground(blank);
        line.addView(face, new LinearLayout.LayoutParams(skin.dp(32), skin.dp(32)));
        wear(face, rule.pkg);

        LinearLayout names = skin.column();
        names.addView(skin.label(labelOf(rule.pkg), skin.bold, 14));
        TextView pkg = skin.label(rule.pkg, skin.muted, 11);
        pkg.setTypeface(Typeface.MONOSPACE);
        names.addView(pkg);

        LinearLayout.LayoutParams namesAt = new LinearLayout.LayoutParams(
                0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f);
        namesAt.leftMargin = skin.dp(12);
        line.addView(names, namesAt);

        TextView remove = skin.cross(30);
        remove.setOnClickListener(new View.OnClickListener() {
            @Override
            public void onClick(View v) {
                drop(rule);
            }
        });
        line.addView(remove, new LinearLayout.LayoutParams(skin.dp(30), skin.dp(30)));
        block.addView(line);

        final LinearLayout picker = skin.column();
        LinearLayout.LayoutParams pickAt = new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT);
        pickAt.topMargin = skin.dp(10);
        block.addView(picker, pickAt);
        fill(picker, rule);

        return block;
    }

    private void fill(final LinearLayout picker, final Rule rule) {
        picker.removeAllViews();
        picker.addView(skin.segments(NAMES, indexOf(rule.role), new Skin.Pick() {
            @Override
            public void at(int index) {
                if (ROLES[index].equals(rule.role)) {
                    return;
                }
                rule.role = ROLES[index];
                fill(picker, rule);
                save();
            }
        }));
    }

    private static int indexOf(String role) {
        return indexIn(ROLES, role);
    }

    private static int indexIn(String[] among, String role) {
        for (int i = 0; i < among.length; i++) {
            if (among[i].equals(role)) {
                return i;
            }
        }
        return among.length - 1;
    }

    private void drop(final Rule rule) {
        LinearLayout wrap = skin.column();
        wrap.setPadding(skin.dp(20), skin.dp(20), skin.dp(20), skin.dp(16));

        LinearLayout head = new LinearLayout(host);
        head.setOrientation(LinearLayout.HORIZONTAL);
        head.setGravity(Gravity.CENTER_VERTICAL);

        ImageView face = new ImageView(host);
        GradientDrawable blank = new GradientDrawable();
        blank.setColor(skin.idle);
        blank.setCornerRadius(skin.dp(8));
        face.setBackground(blank);
        head.addView(face, new LinearLayout.LayoutParams(skin.dp(36), skin.dp(36)));
        wear(face, rule.pkg);

        LinearLayout names = skin.column();
        names.addView(skin.label(labelOf(rule.pkg), skin.bold, 16));
        TextView pkg = skin.label(rule.pkg, skin.muted, 11);
        pkg.setTypeface(Typeface.MONOSPACE);
        names.addView(pkg);

        LinearLayout.LayoutParams namesAt = new LinearLayout.LayoutParams(
                0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f);
        namesAt.leftMargin = skin.dp(12);
        head.addView(names, namesAt);
        wrap.addView(head);

        TextView ask = skin.label("Удалить правило?", skin.text, 15);
        LinearLayout.LayoutParams askAt = new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT);
        askAt.topMargin = skin.dp(18);
        wrap.addView(ask, askAt);

        LinearLayout feet = new LinearLayout(host);
        feet.setOrientation(LinearLayout.HORIZONTAL);
        feet.setGravity(Gravity.END);
        LinearLayout.LayoutParams feetAt = new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT);
        feetAt.topMargin = skin.dp(20);
        wrap.addView(feet, feetAt);

        TextView keep = skin.button("Отмена", skin.idle);
        TextView kill = skin.button("Удалить", skin.bad);
        kill.setTextColor(0xFFFFFFFF);
        LinearLayout.LayoutParams killAt = new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.WRAP_CONTENT, LinearLayout.LayoutParams.WRAP_CONTENT);
        killAt.leftMargin = skin.dp(10);
        feet.addView(keep);
        feet.addView(kill, killAt);

        final AlertDialog dialog = new AlertDialog.Builder(host, R.style.RoundDialog)
                .setView(wrap)
                .create();

        keep.setOnClickListener(new View.OnClickListener() {
            @Override
            public void onClick(View v) {
                dialog.dismiss();
            }
        });
        kill.setOnClickListener(new View.OnClickListener() {
            @Override
            public void onClick(View v) {
                dialog.dismiss();
                rules.remove(rule);
                redraw();
                save();
            }
        });

        dialog.show();
        skin.frame(dialog);
    }


    private void pickApp() {
        if (catalogue != null) {
            offer(catalogue);
            return;
        }

        new Thread(new Runnable() {
            @Override
            public void run() {
                final List<App> found = installed();
                host.runOnUiThread(new Runnable() {
                    @Override
                    public void run() {
                        catalogue = found;
                        offer(found);
                    }
                });
            }
        }).start();
    }

    private void offer(final List<App> apps) {
        LinearLayout wrap = skin.column();
        wrap.setPadding(skin.dp(16), skin.dp(6), skin.dp(16), 0);

        final EditText search = new EditText(host);
        search.setHint("поиск");
        search.setTextColor(skin.bold);
        search.setHintTextColor(skin.muted);
        search.setTextSize(TypedValue.COMPLEX_UNIT_SP, 14);
        search.setSingleLine(true);
        search.setPadding(skin.dp(14), skin.dp(11), skin.dp(14), skin.dp(11));

        GradientDrawable field = new GradientDrawable();
        field.setColor(skin.field);
        field.setCornerRadius(skin.dp(14));
        field.setStroke(Math.max(1, skin.dp(1) / 2), skin.edge);
        search.setBackground(field);
        wrap.addView(search);

        final LinearLayout found = skin.column();
        Scroller scroll = new Scroller(host, skin);
        scroll.setClipChildren(false);
        scroll.setClipToPadding(false);
        scroll.addView(found);
        LinearLayout.LayoutParams listAt = new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, skin.dp(380));
        listAt.topMargin = skin.dp(10);
        wrap.addView(scroll, listAt);

        LinearLayout feet = new LinearLayout(host);
        feet.setOrientation(LinearLayout.HORIZONTAL);
        feet.setGravity(Gravity.END);
        feet.setPadding(0, skin.dp(12), 0, skin.dp(14));

        TextView shut = skin.button("Отмена", skin.idle);
        feet.addView(shut);
        wrap.addView(feet);

        final AlertDialog dialog = new AlertDialog.Builder(host, R.style.RoundDialog)
                .setTitle("Приложение")
                .setView(wrap)
                .create();

        shut.setOnClickListener(new View.OnClickListener() {
            @Override
            public void onClick(View v) {
                dialog.dismiss();
            }
        });

        fill(found, apps, "", dialog);
        search.addTextChangedListener(new TextWatcher() {
            @Override
            public void beforeTextChanged(CharSequence s, int a, int b, int c) {
            }

            @Override
            public void onTextChanged(CharSequence s, int a, int b, int c) {
                fill(found, apps, s.toString(), dialog);
            }

            @Override
            public void afterTextChanged(Editable s) {
            }
        });

        dialog.show();
        skin.frame(dialog);
    }

    private void fill(LinearLayout into, List<App> apps, String needle, final AlertDialog dialog) {
        into.removeAllViews();
        String query = needle.trim().toLowerCase();
        int shown = 0;

        for (final App app : apps) {
            if (!query.isEmpty()
                    && !app.label.toLowerCase().contains(query)
                    && !app.pkg.toLowerCase().contains(query)) {
                continue;
            }
            if (shown++ > 300) {
                break;
            }

            final boolean ruled = ruled(app.pkg);

            LinearLayout item = new LinearLayout(host);
            item.setOrientation(LinearLayout.HORIZONTAL);
            item.setGravity(Gravity.CENTER_VERTICAL);
            item.setPadding(skin.dp(8), skin.dp(8), skin.dp(18), skin.dp(8));

            ImageView face = new ImageView(host);
            GradientDrawable blank = new GradientDrawable();
            blank.setColor(skin.idle);
            blank.setCornerRadius(skin.dp(8));
            face.setBackground(blank);
            item.addView(face, new LinearLayout.LayoutParams(skin.dp(32), skin.dp(32)));
            wear(face, app.pkg);

            LinearLayout names = skin.column();
            names.addView(skin.label(app.label, skin.bold, 14));
            TextView pkg = skin.label(app.pkg, skin.muted, 11);
            pkg.setTypeface(Typeface.MONOSPACE);
            names.addView(pkg);

            LinearLayout.LayoutParams namesAt = new LinearLayout.LayoutParams(
                    0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f);
            namesAt.leftMargin = skin.dp(12);
            item.addView(names, namesAt);

            if (ruled) {
                TextView tag = skin.label("в правилах", skin.muted, 11);
                tag.setPadding(skin.dp(9), skin.dp(4), skin.dp(9), skin.dp(4));
                GradientDrawable badge = new GradientDrawable();
                badge.setColor(skin.idle);
                badge.setCornerRadius(skin.dpf(9f));
                tag.setBackground(badge);
                item.addView(tag);
                item.setAlpha(0.45f);
            }

            GradientDrawable seat = new GradientDrawable();
            seat.setColor(0x00000000);
            seat.setCornerRadius(skin.dp(12));
            item.setBackground(skin.touchable(seat));
            if (!ruled) {
                item.setOnClickListener(new View.OnClickListener() {
                    @Override
                    public void onClick(View v) {
                        dialog.dismiss();
                        adopt(app.pkg);
                    }
                });
            }

            LinearLayout.LayoutParams itemAt = new LinearLayout.LayoutParams(
                    LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT);
            itemAt.bottomMargin = skin.dp(2);
            into.addView(item, itemAt);
        }

        if (shown == 0) {
            TextView empty = skin.label("ничего не нашлось", skin.muted, 14);
            empty.setGravity(Gravity.CENTER);
            empty.setPadding(0, skin.dp(28), 0, skin.dp(28));
            into.addView(empty);
        }
    }

    private void wear(final ImageView face, final String pkg) {
        Drawable known = faces.get(pkg);
        if (known != null) {
            face.setImageDrawable(known);
            return;
        }
        if (loaders == null) {
            loaders = Executors.newFixedThreadPool(2);
        }
        loaders.execute(new Runnable() {
            @Override
            public void run() {
                Drawable got;
                try {
                    got = host.getPackageManager().getApplicationIcon(pkg);
                } catch (Exception e) {
                    return;
                }
                final Drawable ready = got;
                host.runOnUiThread(new Runnable() {
                    @Override
                    public void run() {
                        faces.put(pkg, ready);
                        face.setImageDrawable(ready);
                    }
                });
            }
        });
    }

    private boolean ruled(String pkg) {
        for (Rule rule : rules) {
            if (rule.pkg.equals(pkg)) {
                return true;
            }
        }
        return false;
    }

    private void adopt(String pkg) {
        for (Rule rule : rules) {
            if (rule.pkg.equals(pkg)) {
                say("Уже в списке", false);
                return;
            }
        }

        Rule rule = new Rule();
        rule.pkg = pkg;
        rule.role = "tunnel";
        rules.add(rule);
        redraw();
        save();
    }

    private void save() {
        final String body;
        try {
            JSONArray rows = new JSONArray();
            for (Rule rule : rules) {
                JSONObject row = new JSONObject();
                row.put("id", rule.id);
                row.put("process", rule.pkg);
                row.put("role", rule.role);
                rows.put(row);
            }
            JSONObject payload = new JSONObject();
            payload.put("defaultRole", defaultRole);
            payload.put("rules", rows);
            body = payload.toString();
        } catch (Exception e) {
            say(String.valueOf(e.getMessage()), true);
            return;
        }

        new Thread(new Runnable() {
            @Override
            public void run() {
                String problem = null;
                try {
                    Core.client(host).saveRulesJSON(body);
                } catch (Exception e) {
                    problem = e.getMessage();
                }
                final String said = problem;
                host.runOnUiThread(new Runnable() {
                    @Override
                    public void run() {
                        if (said != null) {
                            say(said, true);
                        }
                        loaded = false;
                        render();
                    }
                });
            }
        }).start();
    }

    private List<App> installed() {
        if (catalogue != null) {
            return catalogue;
        }

        PackageManager packages = host.getPackageManager();
        Set<String> launchable = launchable(packages);
        List<App> out = new ArrayList<>();

        for (PackageInfo info : packages.getInstalledPackages(PackageManager.GET_PERMISSIONS)) {
            if (info.applicationInfo == null) {
                continue;
            }
            if (!networked(info) && !launchable.contains(info.packageName)) {
                continue;
            }
            if (info.packageName.equals(host.getPackageName())) {
                continue;
            }

            App app = new App();
            app.pkg = info.packageName;
            app.label = String.valueOf(packages.getApplicationLabel(info.applicationInfo));
            app.system = (info.applicationInfo.flags & ApplicationInfo.FLAG_SYSTEM) != 0;
            out.add(app);
            labels.put(app.pkg, app.label);
        }

        Collections.sort(out, (a, b) -> {
            if (a.system != b.system) {
                return a.system ? 1 : -1;
            }
            return a.label.compareToIgnoreCase(b.label);
        });

        catalogue = out;
        return out;
    }

    private static Set<String> launchable(PackageManager packages) {
        Set<String> out = new HashSet<>();
        Intent home = new Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_LAUNCHER);
        for (ResolveInfo found : packages.queryIntentActivities(home, 0)) {
            if (found.activityInfo != null) {
                out.add(found.activityInfo.packageName);
            }
        }
        return out;
    }

    private static boolean networked(PackageInfo info) {
        if (info.requestedPermissions == null) {
            return false;
        }
        for (String permission : info.requestedPermissions) {
            if ("android.permission.INTERNET".equals(permission)) {
                return true;
            }
        }
        return false;
    }

    private String labelOf(String pkg) {
        String known = labels.get(pkg);
        if (known != null) {
            return known;
        }
        try {
            PackageManager packages = host.getPackageManager();
            String label = String.valueOf(
                    packages.getApplicationLabel(packages.getApplicationInfo(pkg, 0)));
            labels.put(pkg, label);
            return label;
        } catch (Exception e) {
            return pkg;
        }
    }

    private void say(String text, boolean bad) {
    }

    public boolean onResult(int request, int result, Intent data) {
        if (request != SAVE_RULES && request != LOAD_RULES) {
            return false;
        }
        if (result != Activity.RESULT_OK || data == null || data.getData() == null) {
            return true;
        }
        final Uri where = data.getData();
        final boolean saving = request == SAVE_RULES;
        new Thread(new Runnable() {
            @Override
            public void run() {
                String title;
                String text;
                try {
                    if (saving) {
                        String code = Core.client(host).exportRules();
                        try (OutputStream out = host.getContentResolver().openOutputStream(where)) {
                            out.write(code.getBytes(StandardCharsets.UTF_8));
                        }
                        title = "Правила сохранены";
                        text = "Файл можно загрузить обратно в клиенте qd для Android.";
                    } else {
                        ByteArrayOutputStream got = new ByteArrayOutputStream();
                        try (InputStream in = host.getContentResolver().openInputStream(where)) {
                            byte[] chunk = new byte[8192];
                            int n;
                            while ((n = in.read(chunk)) > 0 && got.size() < (4 << 20)) {
                                got.write(chunk, 0, n);
                            }
                        }
                        long count = Core.client(host).importRules(got.toString("UTF-8"));
                        title = "Правила загружены";
                        text = "Правил в файле: " + count + ".";
                    }
                } catch (Exception e) {
                    title = saving ? "Не удалось сохранить" : "Не удалось загрузить";
                    text = String.valueOf(e.getMessage());
                }
                final String shownTitle = title;
                final String shownText = text;
                host.runOnUiThread(new Runnable() {
                    @Override
                    public void run() {
                        notice(shownTitle, shownText);
                        loaded = false;
                        render();
                    }
                });
            }
        }).start();
        return true;
    }

    private void notice(String title, String text) {
        LinearLayout wrap = skin.column();
        wrap.setPadding(skin.dp(20), skin.dp(20), skin.dp(20), skin.dp(16));
        wrap.addView(skin.label(title, skin.bold, 16));

        TextView body = skin.label(text, skin.text, 15);
        LinearLayout.LayoutParams bodyAt = new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT);
        bodyAt.topMargin = skin.dp(12);
        wrap.addView(body, bodyAt);

        LinearLayout feet = new LinearLayout(host);
        feet.setOrientation(LinearLayout.HORIZONTAL);
        feet.setGravity(Gravity.END);
        LinearLayout.LayoutParams feetAt = new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT);
        feetAt.topMargin = skin.dp(20);
        wrap.addView(feet, feetAt);

        TextView fine = skin.button("OK", skin.good);
        fine.setTextColor(0xFFFFFFFF);
        feet.addView(fine);

        final AlertDialog dialog = new AlertDialog.Builder(host, R.style.RoundDialog)
                .setView(wrap)
                .create();
        fine.setOnClickListener(new View.OnClickListener() {
            @Override
            public void onClick(View v) {
                dialog.dismiss();
            }
        });
        dialog.show();
        skin.frame(dialog);
    }


}
