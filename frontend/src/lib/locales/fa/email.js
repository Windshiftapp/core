/**
 * Persian (fa) transactional and notification email copy.
 *
 * Rendered by the Go backend (internal/i18n) via the `t` template function.
 * Placeholders use {name} syntax and must match the English catalog.
 */
export default {
  email: {
    footer: {
      automated: 'این یک ایمیل خودکار است — لطفاً پاسخ ندهید.',
      repliable: 'این ایمیل را دریافت میکنید زیرا درخواست بازی با ما دارید.'
    },
    common: {
      greeting: 'سلام {name}،',
      openInWindshift: 'مشاهده در ویندشیفت',
      manualLinkHint: 'اگر دکمه کار نکرد، این نشانی را در مرورگر خود کپی و جایگذاری کنید:'
    },
    duration: {
      minute: '{count} دقیقه',
      minutes: '{count} دقیقه',
      hour: '{count} ساعت',
      hours: '{count} ساعت',
      day: '{count} روز',
      days: '{count} روز'
    },
    magicLink: {
      subject: 'ورود به پورتال شما',
      heading: 'ورود به پورتال شما',
      body: 'برای ورود روی دکمه زیر کلیک کنید. این پیوند تا {expires} معتبر است.',
      button: 'ورود',
      note: 'اگر شما این پیوند را درخواست نکردهاید، میتوانید این ایمیل را نادیده بگیرید — هیچکس بدون آن نمیتواند وارد شود.'
    },
    verification: {
      subject: 'نشانی ایمیل خود را تأیید کنید',
      heading: 'تأیید ایمیل شما',
      body: 'برای تکمیل راهاندازی حساب خود، لطفاً نشانی ایمیل خود را تأیید کنید.',
      button: 'تأیید ایمیل',
      note: 'این پیوند تا {expires} منقضی میشود. اگر حساب ویندشیفت نساختهاید، میتوانید این ایمیل را نادیده بگیرید.'
    },
    invitation: {
      subject: 'شما به ویندشیفت دعوت شدهاید',
      heading: 'شما به ویندشیفت دعوت شدهاید',
      body: 'برای فعالسازی حساب خود و شروع همکاری با تیم، یک گذرواژه تعیین کنید.',
      button: 'تعیین گذرواژه',
      note: 'این دعوتنامه تا {expires} منقضی میشود. اگر انتظار آن را نداشتید، میتوانید این ایمیل را نادیده بگیرید.'
    },
    notificationBatch: {
      subjectOne: 'ویندشیفت — ۱ اعلان جدید دارید',
      subjectOther: 'ویندشیفت — {count} اعلان جدید دارید',
      headingOne: '۱ اعلان جدید',
      headingOther: '{count} اعلان جدید',
      intro: 'سلام {name}، آنچه تازه است:',
      manage: 'بسامد دریافت این ایمیلها را در تنظیمات اعلان خود مدیریت کنید.'
    },
    mention: {
      subject: '{actor} در {itemKey} به شما اشاره کرد',
      body: '{actor} در {source} در {itemKey} · {itemTitle} به شما اشاره کرد',
      source: {
        comment: 'یک دیدگاه',
        description: 'توضیحات',
        item: 'یک مورد'
      }
    },
    portalReply: {
      subject: 'پاسخ: {subject}',
      body: '{author} در {itemKey} · {itemTitle} پاسخ داد',
      portalLink: 'مشاهده این درخواست در پورتال شما',
      note: 'برای ادامه گفتگو، کافی است به این ایمیل پاسخ دهید.'
    },
    approval: {
      subject: 'درخواست تأیید: {itemKey} — {itemTitle}',
      heading: 'درخواست تأیید',
      body: 'تأیید شما برای {itemKey} · {itemTitle} لازم است.',
      body2: 'برای بررسی و تصمیمگیری روی دکمه زیر کلیک کنید. این پیوند تا {expires} معتبر است.',
      button: 'بررسی تأیید',
      note: 'اگر انتظار این درخواست را نداشتید، میتوانید این ایمیل را نادیده بگیرید.'
    },
    intake: {
      subject: 'درخواست شما را دریافت کردیم ({itemKey})',
      heading: 'درخواست شما را دریافت کردیم',
      body: 'از تماس شما سپاسگزاریم. درخواست شما را با شناسه {itemKey} · {itemTitle} ثبت کردیم.',
      body2: 'تیم ما آن را بررسی میکند و در اسرع وقت با شما تماس میگیرد.',
      body3: 'میتوانید پیشرفت را در پورتال خود پیگیری کرده و جزئیات را اضافه کنید:',
      button: 'مشاهده درخواست شما',
      note: 'برای افزودن اطلاعات بیشتر، کافی است به این ایمیل پاسخ دهید.'
    }
  },
  notification: {
    dueReminder: {
      title: 'یادآوری: {itemKey} بهزودی سررسید میشود',
      message: '{itemTitle} در {dueDate} سررسید میشود'
    },
    mention: {
      title: 'به شما اشاره شد',
      comment: '{actor} در یک دیدگاه در {itemTitle} ({itemKey}) به شما اشاره کرد',
      description: '{actor} در توضیحات {itemTitle} ({itemKey}) به شما اشاره کرد',
      item: '{actor} در {itemTitle} ({itemKey}) به شما اشاره کرد'
    },
    approvalStarted: {
      title: 'تأیید لازم است',
      message: 'تأیید شما برای {itemKey}: {itemTitle} لازم است'
    },
    approvalEscalated: {
      title: 'تأیید به شما ارجاع شد',
      message: 'یک تأیید در {itemKey}: {itemTitle} به شما ارجاع شده است'
    },
    incident: {
      title: 'رخداد: {itemTitle}',
      message: '{itemKey} فعال شده و نیاز به تأیید دریافت دارد'
    }
  }
};
