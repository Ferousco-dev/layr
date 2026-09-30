import { GOVERNING_LAW } from '../../../config/env.js'

const law = GOVERNING_LAW
  ? `These Terms are governed by the laws of ${GOVERNING_LAW}, without regard to its conflict of laws rules, and the courts of ${GOVERNING_LAW} have jurisdiction over any dispute, except where the law of your country gives you the right to bring a claim elsewhere.`
  : 'These Terms are governed by the laws of the country where {operator} is established, without regard to its conflict of laws rules, and the courts of that country have jurisdiction over any dispute, except where the law of your country gives you the right to bring a claim elsewhere.'

export const terms = {
  title: 'Terms and Conditions',
  effective: '30 September 2026',
  summary: [
    'You sign in with your Figma account. We never see your Figma password.',
    'You keep ownership of your designs and of the code made from them.',
    'AI provider keys are optional. You pay your provider, and you can remove a key at any time.',
    'Layr is early software. It can change, and temporary files are deleted automatically, so keep your Figma files as the original.',
    'You can delete your account yourself, at any time, from the Profile page.',
  ],
  sections: [
    {
      id: 'about',
      title: 'About these terms',
      body: [
        'These Terms and Conditions ("Terms") govern your use of Layr, a web service at layr.appmd.dev that connects to your Figma account, imports designs you choose, and prepares them for turning into code (the "Service"). {operator} ("Layr", "we", "us") runs the Service.',
        'By signing in or using the Service you agree to these Terms and to our {privacy}. If you do not agree, please do not use the Service.',
      ],
    },
    {
      id: 'service',
      title: 'The Service',
      body: [
        'At the time of writing, the Service lets you:',
        { list: ['sign in with your Figma account;', 'create projects and import Figma designs that you have access to;', 'preview the screens that were imported;', 'save optional keys for AI providers;', 'delete projects and your account.'] },
        'Other features, such as generating code and connecting GitHub, are planned. They are not part of the Service until they are released, and when they are, these Terms apply to them too.',
        'Layr is under active development. Features can change, be limited, or contain errors.',
      ],
    },
    {
      id: 'eligibility',
      title: 'Who can use Layr',
      body: [
        'You must be at least 18 years old, or the age of majority where you live, and able to form a binding agreement. If you use Layr for an organisation, you confirm that you have authority to accept these Terms for it.',
      ],
    },
    {
      id: 'account',
      title: 'Your account',
      body: [
        'You sign in only through Figma. Layr never asks for or sees your Figma password. Each Layr account is linked to one Figma account.',
        'You are responsible for what happens under your account, so keep your devices and Figma account secure. If you think someone else has used your account, log out, review your Figma account, and tell us at {email}.',
        'You can stop using Layr and delete your account at any time from the Profile page. How that works is described in our {privacy}.',
      ],
    },
    {
      id: 'your-content',
      title: 'Your designs and content',
      body: [
        'You keep all rights in your designs, files, and other content ("Your Content"). We do not claim ownership of it.',
        'To run the Service for you, you give us a limited, non-exclusive permission to access, copy, process and temporarily store Your Content, only as needed to fetch it from Figma, prepare previews, convert it, and show you the results. This permission ends when the content is deleted from Layr.',
        'You confirm that you have the right to import the designs you choose: that you own them, or that their owner or your employer or client allows it, and that importing them does not break Figma\'s terms or any agreement you are bound by.',
        'We do not use Your Content or your keys to train machine learning models.',
      ],
    },
    {
      id: 'output',
      title: 'Code and other output',
      body: [
        'Code or other output made from Your Content is yours, and we do not claim rights in it. Output may contain mistakes, may be similar to other code, and may include fonts, images or other assets that come with their own licences. Review and test it before you rely on it, and check that you have the right to use everything it contains.',
      ],
    },
    {
      id: 'ai-keys',
      title: 'AI provider keys',
      body: [
        'You may save your own API key for AI providers such as Anthropic, OpenAI and xAI. This is optional.',
        { list: [
          'You are responsible for the key, for all charges and limits from the provider, and for following the provider\'s terms.',
          'We store the key encrypted and show only its last four characters. You can replace or remove it at any time.',
          'When you save a key, we check only that it looks like a key for that provider. We do not contact the provider to confirm that it works.',
          'If you think a key has been exposed, revoke it with the provider straight away.',
        ] },
      ],
    },
    {
      id: 'third-parties',
      title: 'Third-party services',
      body: [
        'Layr works with services we do not control. Figma provides sign-in and your design files, and its own terms and privacy policy apply to that. You can withdraw Layr\'s access at any time in your Figma account settings. In the future, GitHub and the AI providers you choose may also be involved, under their own terms.',
        'We are not responsible for third-party services, or for changes they make that affect Layr.',
      ],
    },
    {
      id: 'acceptable-use',
      title: 'Acceptable use',
      body: [
        'Please do not:',
        { list: [
          'break the law or infringe anyone\'s rights, including by importing designs you have no right to use;',
          'try to access another person\'s account or data, or to get around security, limits or access controls;',
          'probe, scan or test the Service for weaknesses without our written permission;',
          'overload the Service or use automated means to send requests beyond the limits we set;',
          'upload or distribute malware, or use Layr to harm others;',
          'reverse engineer the Service, except where the law allows it;',
          'resell or offer the Service to others without our permission.',
        ] },
        'If you find a security problem, please tell us at {email} before you share it publicly.',
      ],
    },
    {
      id: 'limits',
      title: 'Limits',
      body: [
        'To keep the Service fast and fair, we limit how often you can sign in, save changes and import, and how large a design, image or import can be. Very large files may be refused. Limits can change.',
      ],
    },
    {
      id: 'our-rights',
      title: 'Our intellectual property',
      body: [
        'The Service, its software, design, name and logo belong to {operator} or its licensors. While you follow these Terms, you may use the Service for its intended purpose. This is a limited, revocable, non-exclusive, non-transferable permission. You may not use the Layr name or logo without our permission.',
        'If you send us ideas or feedback, we may use them without obligation or payment to you.',
      ],
    },
    {
      id: 'availability',
      title: 'Availability and changes',
      body: [
        'We provide the Service as it is available. It may be interrupted for maintenance or because of problems we do not control, and we may change or stop features. Where a change would seriously affect you, we will try to tell you in advance.',
        'Imported design files are temporary and are removed automatically after a short time. Layr is not a backup. Your Figma file remains the original, so keep it safe.',
      ],
    },
    {
      id: 'termination',
      title: 'Ending your use',
      body: [
        'You can stop using Layr at any time and delete your account from the Profile page.',
        'We may suspend or end your access if you break these Terms, if your use puts the Service or other people at risk, or if the law requires it. Where we reasonably can, we will tell you why.',
        'Sections that by their nature should continue after your access ends, such as ownership, disclaimers, limits on liability and governing law, will continue.',
      ],
    },
    {
      id: 'disclaimers',
      title: 'Disclaimers',
      body: [
        'The Service is provided "as is" and "as available". To the fullest extent the law allows, we do not promise that it will be uninterrupted or error free, that imported designs or generated output will be accurate or complete, or that the Service will suit your particular purpose. We disclaim all implied warranties, including merchantability, fitness for a particular purpose and non-infringement.',
        'Some places do not allow certain disclaimers. Where that applies, they apply only as far as the law permits.',
      ],
    },
    {
      id: 'liability',
      title: 'Limits on our liability',
      body: [
        'To the fullest extent the law allows, {operator} is not liable for indirect, incidental, special, consequential or punitive damages, or for lost profits, revenue, data or goodwill, arising from your use of the Service.',
        'Our total liability for all claims relating to the Service is limited to the greater of the amount you paid us in the 12 months before the claim and US$100.',
        'Nothing in these Terms limits liability that cannot be limited by law, such as liability for fraud or for death or personal injury caused by negligence, or any rights you have as a consumer that cannot be waived.',
      ],
    },
    {
      id: 'indemnity',
      title: 'Your responsibility for claims',
      body: [
        'To the extent the law allows, you will cover us against claims, losses and costs that arise from Your Content, your use of the Service in breach of these Terms, or your breach of someone else\'s rights.',
      ],
    },
    {
      id: 'law',
      title: 'Governing law and disputes',
      body: [law, 'If you have a concern, please contact us first at {email}. Most issues can be solved quickly and without a dispute.'],
    },
    {
      id: 'changes',
      title: 'Changes to these terms',
      body: [
        'We may update these Terms. The effective date at the top shows the latest version. If a change is significant, we will tell you in the Service or by email before it applies. If you keep using Layr after the change takes effect, you accept the updated Terms. If you do not agree, you can delete your account.',
      ],
    },
    {
      id: 'general',
      title: 'General',
      body: [
        'These Terms and our {privacy} are the whole agreement between you and {operator} about the Service. If a part of these Terms cannot be enforced, the rest still applies. If we do not enforce a right straight away, we have not given it up. You may not transfer your rights under these Terms without our consent; we may transfer ours as part of a reorganisation or sale of the Service. These Terms do not give rights to anyone else.',
      ],
    },
    {
      id: 'contact',
      title: 'Contact',
      body: ['Questions about these Terms? Contact us at {email}.'],
    },
  ],
}
