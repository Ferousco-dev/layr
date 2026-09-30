export const privacy = {
  title: 'Privacy Policy',
  effective: '30 September 2026',
  summary: [
    'We collect only what Layr needs: your Figma profile, the designs you choose to import, and any AI provider keys you choose to save.',
    'Figma access tokens and AI keys are stored encrypted. Saved keys are never shown again, only their last four characters.',
    'We have no ads, no analytics and no tracking cookies. We do not sell your data.',
    'Imported design files are temporary and are removed automatically, by default within 24 hours.',
    'You can delete your account and everything in it yourself, from the Profile page.',
  ],
  sections: [
    {
      id: 'who',
      title: 'Who we are',
      body: [
        '{operator} ("Layr", "we", "us") runs the Layr service at layr.appmd.dev. We decide how and why your personal data is used, so we are the controller of it. This policy explains what we collect, why, and the choices you have. It applies together with our {terms}.',
      ],
    },
    {
      id: 'collect',
      title: 'What we collect',
      body: [
        'From Figma, when you sign in:',
        { list: [
          'your Figma user ID, your name, your email address (if Figma shares one) and a link to your profile picture;',
          'access and refresh tokens, which let Layr read your designs on your behalf.',
        ] },
        'We ask Figma for two read-only permissions: your basic profile (current_user:read) and the contents of files you ask us to import (file_content:read). We use them only to sign you in and import files you choose. Layr cannot change your Figma files.',
        'From your use of Layr:',
        { list: [
          'projects: the names you give them and their dates;',
          'imports: the Figma file link and frame identifiers, the file name and version, status, counts and any error code;',
          'design data: for each import, the Figma data for the frames you choose, the images and vector graphics they use, preview images, and a structured version of the design that Layr creates. These files are temporary (see "How long we keep it");',
          'AI provider keys, if you choose to save them. We keep the key encrypted, plus its last four characters so you can recognise it.',
        ] },
        'From your browser and our servers:',
        { list: [
          'a session cookie that keeps you signed in (see "Cookies");',
          'server logs of each request: a request ID, the method, the page or API route, the result, how long it took, and error codes. Logs do not contain request contents, Figma tokens, session values or AI keys;',
          'your network address, held only in short-lived counters (about a minute) that limit how often requests can be made, so the Service is not abused.',
        ] },
        'We do not collect passwords, payment details or precise location, and Layr does not currently take payments.',
      ],
    },
    {
      id: 'use',
      title: 'How we use it, and why',
      body: [
        { list: [
          'To provide the Service: sign you in, show your projects, fetch and convert the designs you choose, and manage your keys. This is needed to carry out our agreement with you.',
          'To keep Layr secure and reliable: prevent abuse, limit request rates, find and fix faults. This is in our legitimate interest in running a safe service.',
          'To meet legal obligations, and to respond to your requests about your data.',
          'To contact you about the Service when needed, for example a security or account matter.',
        ] },
        'We do not sell your personal data, we do not use it for advertising, and we do not make decisions about you by automated means that have legal or similarly significant effects. We do not use your designs or keys to train machine learning models.',
      ],
    },
    {
      id: 'cookies',
      title: 'Cookies and similar technology',
      body: [
        'Layr sets one cookie, which is essential: a random session code that keeps you signed in. It cannot be read by scripts on the page, is sent only to Layr, is marked Secure when the site is served over HTTPS, and lasts up to 7 days by default or until you log out. We store only a scrambled (hashed) form of it on our side.',
        'We do not use advertising, analytics or other tracking cookies, and we do not use browser storage to track you. Because there is nothing optional to switch off, there is no cookie banner.',
      ],
    },
    {
      id: 'sharing',
      title: 'Who we share it with',
      body: [
        { list: [
          'Figma. We contact Figma to sign you in and to fetch the designs you choose. Figma handles that data under its own privacy policy. Your profile picture is also loaded from Figma\'s servers, so Figma can see that request.',
          'Service providers that host and run Layr, such as server, database and network providers. They handle data for us and only on our instructions.',
          'AI providers, only when a feature you choose sends your content to a provider using your own key. Features that do this are not part of Layr yet; when they are, the provider\'s own terms and privacy policy will also apply, and we will make it clear before anything is sent.',
          'Authorities or other parties, where the law requires it or to protect people\'s safety and rights, and a buyer if Layr is ever sold or reorganised, who must respect this policy.',
        ] },
      ],
    },
    {
      id: 'transfers',
      title: 'Where data is processed',
      body: [
        'Layr and its providers may process data in countries other than yours, including where Figma operates. Where the law requires safeguards for such transfers, we use them, such as contracts with suitable protections.',
      ],
    },
    {
      id: 'retention',
      title: 'How long we keep it',
      body: [
        { list: [
          'Account details and Figma tokens: until you delete your account.',
          'Sessions: they expire after 7 days by default, end when you log out, and are removed when you delete your account.',
          'Projects: until you delete them. A deleted project can be restored for 30 days, and after that it is permanently removed.',
          'Temporary design files (Figma data, images, previews): removed automatically, by default 24 hours after an import, and immediately when you delete your account.',
          'Import records (link, file name, status): kept with the project they belong to.',
          'AI provider keys: until you remove them or delete your account.',
          'Logs: kept for a limited time needed for security and troubleshooting.',
          'Backups, if any: copies disappear as the backups expire.',
        ] },
        'When you delete your account, your profile, sessions, Figma connection, projects, import records and saved keys are permanently deleted, and the temporary files of your imports are cleared. Layr drops its copy of the Figma tokens but cannot cancel Layr\'s access inside Figma. You can do that in your Figma account settings.',
      ],
    },
    {
      id: 'security',
      title: 'How we protect it',
      body: [
        { list: [
          'Data travels over encrypted connections.',
          'Figma tokens and AI keys are encrypted before they are stored, and keys are never sent back to your browser.',
          'Session codes are stored only in hashed form.',
          'Every request is checked so that you can reach only your own projects and data.',
          'Rate limits, size limits and safe handling of downloaded files reduce abuse.',
        ] },
        'No service can be perfectly secure. If you find a weakness, please tell us at {email}. If a breach affects your personal data, we will tell you and the authorities as the law requires.',
      ],
    },
    {
      id: 'rights',
      title: 'Your choices and rights',
      body: [
        'Depending on where you live, laws such as the GDPR or Nigeria\'s Data Protection Act give you rights over your personal data. In general, you can:',
        { list: [
          'ask what data we hold about you and get a copy;',
          'ask us to correct data that is wrong (your name, email and picture come from Figma, so updating them in Figma updates them in Layr the next time you sign in);',
          'delete your data: use "Delete account" on the Profile page for everything at once, "Remove" next to a key to delete only that key, or delete a project;',
          'object to or ask us to limit certain uses, and take back any consent you gave;',
          'complain to your data protection authority.',
        ] },
        'To use a right that you cannot use yourself in the app, email us at {email}. We may need to confirm it is you, and we aim to reply within 30 days.',
      ],
    },
    {
      id: 'links',
      title: 'Other websites',
      body: ['Layr may link to other websites, including Figma. We are not responsible for their privacy practices, so please read their policies.'],
    },
    {
      id: 'changes',
      title: 'Changes to this policy',
      body: ['We may update this policy. The effective date at the top shows the latest version. If a change is significant, we will tell you in the Service or by email before it applies.'],
    },
    {
      id: 'contact',
      title: 'Contact',
      body: ['Questions or requests about your data? Contact us at {email}.'],
    },
  ],
}
